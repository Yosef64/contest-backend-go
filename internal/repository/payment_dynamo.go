package repository

import (
	"context"
	"errors"
	"fmt"
	"time"
	"victor-contest-go/internal/domain"
	"victor-contest-go/internal/usecase"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
)

type dynamoDBPaymentRepository struct {
	db        *dynamodb.Client
	tableName string
}

// ListAll implements usecase.PaymentRepository.
func (r *dynamoDBPaymentRepository) ListAll() ([]domain.PaymentRequest, error) {
	var payments []domain.PaymentRequest
	gsi1PK, err := attributevalue.Marshal("PAYMENT_REQUEST")
	if err != nil {
		return nil, err
	}

	out, err := r.db.Query(context.TODO(), &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		IndexName:              aws.String("GSI1PK-user_id-index"),
		KeyConditionExpression: aws.String("GSI1PK = :gsi1pk AND #st = :user_id"),
		ExpressionAttributeNames: map[string]string{
			"#st": "user_id",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":gsi1pk":  gsi1PK,
			":user_id": &types.AttributeValueMemberS{Value: "112pay"},
		},
	})
	if err != nil {
		return nil, err
	}
	if err := attributevalue.UnmarshalListOfMaps(out.Items, &payments); err != nil {
		return nil, fmt.Errorf("failed to unmarshal payments: %w", err)
	}
	return payments, nil
}

func NewDynamoDBPaymentRepository(region string, tableName string) usecase.PaymentRepository {
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region),
	)
	if err != nil {
		panic("unable to load AWS SDK config: " + err.Error())
	}
	return &dynamoDBPaymentRepository{
		db:        dynamodb.NewFromConfig(cfg),
		tableName: tableName,
	}
}

func (r *dynamoDBPaymentRepository) Create(req *domain.PaymentRequest) error {
	if req.ID == "" {
		req.ID = uuid.New().String()
	}
	req.GSI1PK = "PAYMENT_REQUEST"
	item, err := attributevalue.MarshalMap(req)
	if err != nil {
		return fmt.Errorf("failed to marshal payment request: %w", err)
	}

	_, err = r.db.PutItem(context.TODO(), &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      item,
	})
	if err != nil {
		return fmt.Errorf("failed to put item in dynamodb: %w", err)
	}
	return nil
}

func (r *dynamoDBPaymentRepository) GetByID(id string) (*domain.PaymentRequest, error) {
	key, err := attributevalue.MarshalMap(map[string]string{"id": id})
	if err != nil {
		return nil, err
	}

	out, err := r.db.GetItem(context.TODO(), &dynamodb.GetItemInput{
		TableName: aws.String(r.tableName),
		Key:       key,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get item: %w", err)
	}
	if out.Item == nil {
		return nil, errors.New("payment request not found")
	}

	var req domain.PaymentRequest
	if err := attributevalue.UnmarshalMap(out.Item, &req); err != nil {
		return nil, fmt.Errorf("failed to unmarshal item: %w", err)
	}
	return &req, nil
}

func (r *dynamoDBPaymentRepository) UpdateStatus(id string, newStatus domain.PaymentStatus, reason string) error {
	// ✅ Use the full composite key
	key, err := attributevalue.MarshalMap(map[string]string{
		"id": id,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal key: %w", err)
	}

	updateExpression := "SET #status = :status, #updatedAt = :updatedAt, #reason = :reason"
	expressionAttributeNames := map[string]string{
		"#status":    "status",
		"#updatedAt": "updated_at",
		"#reason":    "reason",
	}

	expressionAttributeValues, err := attributevalue.MarshalMap(map[string]any{
		":status":    newStatus,
		":updatedAt": time.Now().UTC(),
		":reason":    aws.String(reason),
	})
	if err != nil {
		return fmt.Errorf("failed to marshal base values: %w", err)
	}

	if reason != "" {
		updateExpression += ", #rejectionReason = :rejectionReason"
		expressionAttributeNames["#rejectionReason"] = "rejection_reason"

		reasonValue, _ := attributevalue.Marshal(reason)
		expressionAttributeValues[":rejectionReason"] = reasonValue
	}

	_, err = r.db.UpdateItem(context.TODO(), &dynamodb.UpdateItemInput{
		TableName:                 aws.String(r.tableName),
		Key:                       key,
		UpdateExpression:          aws.String(updateExpression),
		ExpressionAttributeNames:  expressionAttributeNames,
		ExpressionAttributeValues: expressionAttributeValues,
	})

	if err != nil {
		return fmt.Errorf("failed to update item: %w", err)
	}
	return nil
}

// ListByStatus requires a Global Secondary Index (GSI) on the 'status' attribute.
// GSI Name: 'StatusIndex'
// Partition Key: 'status'
func (r *dynamoDBPaymentRepository) ListByStatus(status domain.PaymentStatus) ([]domain.PaymentRequest, error) {
	var payments []domain.PaymentRequest
	st, err := attributevalue.Marshal(status)
	if err != nil {
		return nil, err
	}
	gsi1PK, err := attributevalue.Marshal("PAYMENT_REQUEST")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal GSI PK: %w", err)
	}
	out, err := r.db.Query(context.TODO(), &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		IndexName:              aws.String("GSI1PK-status-index"),
		KeyConditionExpression: aws.String("GSI1PK = :gsi1pk AND #st = :status"),
		ExpressionAttributeNames: map[string]string{
			"#st": "status",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":gsi1pk": gsi1PK,
			":status": st,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query by status: %w", err)
	}

	if err := attributevalue.UnmarshalListOfMaps(out.Items, &payments); err != nil {
		return nil, fmt.Errorf("failed to unmarshal payments: %w", err)
	}
	return payments, nil
}

// ListByUser requires a Global Secondary Index (GSI) on the 'user_id' attribute.
// GSI Name: 'UserIndex'
// Partition Key: 'user_id'
func (r *dynamoDBPaymentRepository) ListByUser(userID string) ([]domain.PaymentRequest, error) {
	var payments []domain.PaymentRequest
	gsi1PK, err := attributevalue.Marshal("PAYMENT_REQUEST")
	if err != nil {
		return nil, err
	}

	out, err := r.db.Query(context.TODO(), &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		IndexName:              aws.String("GSI1PK-user_id-index"),
		KeyConditionExpression: aws.String("GSI1PK = :gsi1pk AND #st = :user_id"),
		ExpressionAttributeNames: map[string]string{
			"#st": "user_id",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":gsi1pk":  gsi1PK,
			":user_id": &types.AttributeValueMemberS{Value: userID},
		},
	})
	if err != nil {
		return nil, err
	}
	if err := attributevalue.UnmarshalListOfMaps(out.Items, &payments); err != nil {
		return nil, fmt.Errorf("failed to unmarshal payments: %w", err)
	}
	return payments, nil
}

// ListExpired uses the new sparse GSI.
// GSI Name: 'ExpirationIndex'
// Partition Key: 'expirationDate'
func (r *dynamoDBPaymentRepository) ListExpired(now time.Time) ([]domain.PaymentRequest, error) {
	var payments []domain.PaymentRequest
	nowStr, err := attributevalue.Marshal(now)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal current time: %w", err)
	}
	gsi1PK, err := attributevalue.Marshal("PAYMENT_REQUEST")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal GSI PK: %w", err)
	}
	out, err := r.db.Query(context.TODO(), &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		IndexName:              aws.String("GSI1PK-expirationDate-index"),
		KeyConditionExpression: aws.String("GSI1PK = :gsi1pk AND expirationDate < :now"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":gsi1pk": gsi1PK,
			":now":    nowStr,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query for expired payments: %w", err)
	}

	if err := attributevalue.UnmarshalListOfMaps(out.Items, &payments); err != nil {
		return nil, fmt.Errorf("failed to unmarshal expired payments: %w", err)
	}
	return payments, nil
}
