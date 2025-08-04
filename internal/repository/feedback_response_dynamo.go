package repository

import (
	"context"
	"victor-contest-go/internal/domain"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
)

type FeedbackResponseDynamoRepository struct {
	db        *dynamodb.Client
	tableName string
}

func NewFeedbackResponseDynamoRepository(region string, tablename string) *FeedbackResponseDynamoRepository {
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region),
	)
	if err != nil {
		panic("unable to load AWS SDK config: " + err.Error())
	}
	return &FeedbackResponseDynamoRepository{
		db:        dynamodb.NewFromConfig(cfg),
		tableName: tablename,
	}
}

func (r *FeedbackResponseDynamoRepository) AddFeedbackResponse(response domain.FeedbackResponse) (string, error) {
	if response.ID == "" {
		response.ID = uuid.New().String()
	}
	item, err := attributevalue.MarshalMap(response)
	if err != nil {
		return "", err
	}
	_, err = r.db.PutItem(context.TODO(), &dynamodb.PutItemInput{
		TableName: &r.tableName,
		Item:      item,
	})
	if err != nil {
		return "", err
	}
	return response.ID, nil
}

func (r *FeedbackResponseDynamoRepository) UpdateFeedbackResponse(id string, update domain.FeedbackResponse) error {
	update.ID = id
	item, err := attributevalue.MarshalMap(update)
	if err != nil {
		return err
	}
	_, err = r.db.PutItem(context.TODO(), &dynamodb.PutItemInput{
		TableName: &r.tableName,
		Item:      item,
	})
	return err
}

func (r *FeedbackResponseDynamoRepository) DeleteFeedbackResponse(id string) error {
	key, err := attributevalue.MarshalMap(map[string]string{"id": id})
	if err != nil {
		return err
	}
	_, err = r.db.DeleteItem(context.TODO(), &dynamodb.DeleteItemInput{
		TableName: &r.tableName,
		Key:       key,
	})
	return err
}

func (r *FeedbackResponseDynamoRepository) GetFeedbackResponseByID(id string) (*domain.FeedbackResponse, error) {
	key, err := attributevalue.MarshalMap(map[string]string{"id": id})
	if err != nil {
		return nil, err
	}
	out, err := r.db.GetItem(context.TODO(), &dynamodb.GetItemInput{
		TableName: &r.tableName,
		Key:       key,
	})
	if err != nil {
		return nil, err
	}
	if out.Item == nil {
		return nil, nil
	}
	var response domain.FeedbackResponse
	err = attributevalue.UnmarshalMap(out.Item, &response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}

func (r *FeedbackResponseDynamoRepository) GetAllFeedbackResponses() ([]domain.FeedbackResponse, error) {
	out, err := r.db.Scan(context.TODO(), &dynamodb.ScanInput{
		TableName: &r.tableName,
	})
	if err != nil {
		return nil, err
	}
	var responses []domain.FeedbackResponse
	err = attributevalue.UnmarshalListOfMaps(out.Items, &responses)
	if err != nil {
		return nil, err
	}
	return responses, nil
}

func (r *FeedbackResponseDynamoRepository) GetFeedbackResponsesByStudent(studentID string) ([]domain.FeedbackResponse, error) {
	studentIDVal, _ := attributevalue.Marshal(studentID)
	out, err := r.db.Scan(context.TODO(), &dynamodb.ScanInput{
		TableName:        &r.tableName,
		FilterExpression: aws.String("student_id = :student_id"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":student_id": studentIDVal,
		},
	})
	if err != nil {
		return nil, err
	}
	var responses []domain.FeedbackResponse
	err = attributevalue.UnmarshalListOfMaps(out.Items, &responses)
	if err != nil {
		return nil, err
	}
	return responses, nil
}

func (r *FeedbackResponseDynamoRepository) GetFeedbackResponsesByQuestion(questionID string) ([]domain.FeedbackResponse, error) {
	// This is a more complex query since question_responses is a map
	// We'll need to scan and filter in application code
	out, err := r.db.Scan(context.TODO(), &dynamodb.ScanInput{
		TableName: &r.tableName,
	})
	if err != nil {
		return nil, err
	}
	var allResponses []domain.FeedbackResponse
	err = attributevalue.UnmarshalListOfMaps(out.Items, &allResponses)
	if err != nil {
		return nil, err
	}
	
	// Filter responses that contain the specific question
	var filteredResponses []domain.FeedbackResponse
	for _, response := range allResponses {
		if _, exists := response.QuestionResponses[questionID]; exists {
			filteredResponses = append(filteredResponses, response)
		}
	}
	return filteredResponses, nil
} 