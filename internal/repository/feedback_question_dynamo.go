package repository

import (
	"context"
	"fmt"
	"time"
	"victor-contest-go/internal/domain"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
)

type FeedbackQuestionDynamoRepository struct {
	db        *dynamodb.Client
	tableName string
}

func NewFeedbackQuestionDynamoRepository(region string, tablename string) *FeedbackQuestionDynamoRepository {
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region),
	)
	if err != nil {
		panic("unable to load AWS SDK config: " + err.Error())
	}
	return &FeedbackQuestionDynamoRepository{
		db:        dynamodb.NewFromConfig(cfg),
		tableName: tablename,
	}
}

func (r *FeedbackQuestionDynamoRepository) AddFeedbackQuestion(question domain.FeedbackQuestion) (string, error) {
	if question.ID == "" {
		question.ID = uuid.New().String()
	}
	item, err := attributevalue.MarshalMap(question)
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
	return question.ID, nil
}

func (r *FeedbackQuestionDynamoRepository) UpdateFeedbackQuestion(id string, update domain.FeedbackQuestion) error {
	// First, get the existing question to preserve the ID and timestamps
	existingQuestion, err := r.GetFeedbackQuestionByID(id)
	if err != nil {
		return err
	}
	if existingQuestion == nil {
		return fmt.Errorf("question not found")
	}

	// Update the fields with the new data
	existingQuestion.Question = update.Question
	existingQuestion.Options = update.Options
	existingQuestion.AdminID = update.AdminID
	existingQuestion.IsActive = update.IsActive
	existingQuestion.UpdatedAt = time.Now()

	// Save the updated question
	item, err := attributevalue.MarshalMap(existingQuestion)
	if err != nil {
		return err
	}
	_, err = r.db.PutItem(context.TODO(), &dynamodb.PutItemInput{
		TableName: &r.tableName,
		Item:      item,
	})
	return err
}

func (r *FeedbackQuestionDynamoRepository) DeleteFeedbackQuestion(id string) error {
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

func (r *FeedbackQuestionDynamoRepository) GetFeedbackQuestionByID(id string) (*domain.FeedbackQuestion, error) {
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
	var question domain.FeedbackQuestion
	err = attributevalue.UnmarshalMap(out.Item, &question)
	if err != nil {
		return nil, err
	}
	return &question, nil
}

func (r *FeedbackQuestionDynamoRepository) GetAllFeedbackQuestions() ([]domain.FeedbackQuestion, error) {
	out, err := r.db.Scan(context.TODO(), &dynamodb.ScanInput{
		TableName: &r.tableName,
	})
	if err != nil {
		return nil, err
	}
	var questions []domain.FeedbackQuestion
	err = attributevalue.UnmarshalListOfMaps(out.Items, &questions)
	if err != nil {
		return nil, err
	}
	return questions, nil
}

func (r *FeedbackQuestionDynamoRepository) GetActiveFeedbackQuestions() ([]domain.FeedbackQuestion, error) {
	isActiveVal, _ := attributevalue.Marshal(true)
	out, err := r.db.Scan(context.TODO(), &dynamodb.ScanInput{
		TableName:        &r.tableName,
		FilterExpression: aws.String("is_active = :is_active"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":is_active": isActiveVal,
		},
	})
	if err != nil {
		return nil, err
	}
	var questions []domain.FeedbackQuestion
	err = attributevalue.UnmarshalListOfMaps(out.Items, &questions)
	if err != nil {
		return nil, err
	}
	return questions, nil
}

func (r *FeedbackQuestionDynamoRepository) GetFeedbackQuestionsByAdmin(adminID string) ([]domain.FeedbackQuestion, error) {
	adminIDVal, _ := attributevalue.Marshal(adminID)
	out, err := r.db.Scan(context.TODO(), &dynamodb.ScanInput{
		TableName:        &r.tableName,
		FilterExpression: aws.String("admin_id = :admin_id"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":admin_id": adminIDVal,
		},
	})
	if err != nil {
		return nil, err
	}
	var questions []domain.FeedbackQuestion
	err = attributevalue.UnmarshalListOfMaps(out.Items, &questions)
	if err != nil {
		return nil, err
	}
	return questions, nil
}
