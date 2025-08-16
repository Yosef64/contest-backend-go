package repository

import (
	"context"
	"fmt"
	"victor-contest-go/internal/domain"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/google/uuid"
)

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

type QuestionDynamoRepository struct {
	db        *dynamodb.Client
	tableName string
}

func NewQuestionDynamoRepository(region string, tablename string) *QuestionDynamoRepository {
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region),
	)
	if err != nil {
		panic("unable to load AWS SDK config: " + err.Error())
	}
	return &QuestionDynamoRepository{
		db:        dynamodb.NewFromConfig(cfg),
		tableName: tablename,
	}
}

func (r *QuestionDynamoRepository) AddQuestion(question domain.Question) (string, error) {
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

func (r *QuestionDynamoRepository) UpdateQuestion(id string, update domain.Question) error {
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

func (r *QuestionDynamoRepository) DeleteQuestion(id string) error {
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

func (r *QuestionDynamoRepository) GetQuestionByID(id string) (*domain.Question, error) {
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
	var question domain.Question
	err = attributevalue.UnmarshalMap(out.Item, &question)
	if err != nil {
		return nil, err
	}
	return &question, nil
}

func (r *QuestionDynamoRepository) GetAllQuestions() ([]domain.Question, error) {
	out, err := r.db.Scan(context.TODO(), &dynamodb.ScanInput{
		TableName: &r.tableName,
	})
	if err != nil {
		return nil, err
	}

	fmt.Printf("Raw DynamoDB questions scan result: %d items\n", len(out.Items))

	var questions []domain.Question
	err = attributevalue.UnmarshalListOfMaps(out.Items, &questions)
	if err != nil {
		return nil, err
	}

	fmt.Printf("Unmarshaled questions count: %d\n", len(questions))
	for i, q := range questions {
		fmt.Printf("Question %d: ID=%s, Text=%s\n", i, q.ID, q.QuestionText[:min(len(q.QuestionText), 50)])
	}

	return questions, nil
}
