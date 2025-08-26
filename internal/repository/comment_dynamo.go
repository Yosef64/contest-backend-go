package repository

import (
	"context"
	"time"
	"victor-contest-go/internal/domain"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
)

type CommentDynamoRepository struct {
	db        *dynamodb.Client
	tableName string
}

func NewCommentDynamoRepository(region, table string) *CommentDynamoRepository {
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
	if err != nil {
		panic("unable to load AWS SDK config: " + err.Error())
	}
	return &CommentDynamoRepository{db: dynamodb.NewFromConfig(cfg), tableName: table}
}

func (r *CommentDynamoRepository) Create(comment domain.Comment) (string, error) {
	comment.ID = uuid.New().String()
	comment.CreatedAt = time.Now().UTC()
	comment.UpdatedAt = time.Now().UTC()

	// Set default avatar if not provided
	if comment.Avatar == "" {
		comment.Avatar = "https://via.placeholder.com/40"
	}

	item, err := attributevalue.MarshalMap(comment)
	if err != nil {
		return "", err
	}

	_, err = r.db.PutItem(context.TODO(), &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      item,
	})

	return comment.ID, err
}

func (r *CommentDynamoRepository) ListByArticleID(articleID string) ([]domain.Comment, error) {
	input := &dynamodb.QueryInput{
		TableName: aws.String(r.tableName),
		IndexName: aws.String("ArticleIDIndex"), // Assuming you have a GSI on articleId
		KeyConditionExpression: aws.String("articleId = :articleId"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":articleId": &types.AttributeValueMemberS{Value: articleID},
		},
		ScanIndexForward: aws.Bool(false), // Sort by createdAt descending (newest first)
	}

	result, err := r.db.Query(context.TODO(), input)
	if err != nil {
		return nil, err
	}

	var comments []domain.Comment
	err = attributevalue.UnmarshalListOfMaps(result.Items, &comments)
	if err != nil {
		return nil, err
	}

	return comments, nil
}

// Fallback method if GSI is not available - scans the table (less efficient)
func (r *CommentDynamoRepository) ListByArticleIDScan(articleID string) ([]domain.Comment, error) {
	input := &dynamodb.ScanInput{
		TableName: aws.String(r.tableName),
		FilterExpression: aws.String("articleId = :articleId"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":articleId": &types.AttributeValueMemberS{Value: articleID},
		},
	}

	result, err := r.db.Scan(context.TODO(), input)
	if err != nil {
		return nil, err
	}

	var comments []domain.Comment
	err = attributevalue.UnmarshalListOfMaps(result.Items, &comments)
	if err != nil {
		return nil, err
	}

	return comments, nil
}
