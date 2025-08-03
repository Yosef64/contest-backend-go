package repository

import (
	"context"
	"log"
	"strings"
	"victor-contest-go/internal/domain"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
)

type SubmissionDynamoRepository struct {
	db        *dynamodb.Client
	tableName string
}


func NewSubmissionDynamoRepository(region string, tablename string) *SubmissionDynamoRepository {
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region),
	)
	if err != nil {
		panic("unable to load AWS SDK config: " + err.Error())
	}
	return &SubmissionDynamoRepository{
		db:        dynamodb.NewFromConfig(cfg),
		tableName: tablename,
	}
}

func (r *SubmissionDynamoRepository) AddSubmission(submission domain.Submission) (string, error) {
	if submission.ID == "" {
		submission.ID = uuid.New().String()
	}
	item, err := attributevalue.MarshalMap(submission)
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
	return submission.ID, nil
}

func (r *SubmissionDynamoRepository) GetSubmissionByID(id string) (*domain.Submission, error) {
	contestId :=  strings.Split(id, "#")[0]
	key, err := attributevalue.MarshalMap(map[string]string{
	"id":         id,
	"contest_id": contestId,
})
	if err != nil {
		return nil, err
	}
	out, err := r.db.GetItem(context.TODO(), &dynamodb.GetItemInput{
		TableName: &r.tableName,
		Key:       key,
	})
	if err != nil {
		log.Printf("this is the error %s",id)

		return nil, err
	}

	if out.Item == nil {
		log.Printf("this is the error")
		return nil, nil
	}
	var submission domain.Submission
	err = attributevalue.UnmarshalMap(out.Item, &submission)
	if err != nil {
		return nil, err
	}
	return &submission, nil
}

func (r *SubmissionDynamoRepository) GetAllSubmissions() ([]domain.Submission, error) {
	out, err := r.db.Scan(context.TODO(), &dynamodb.ScanInput{
		TableName: &r.tableName,
	})
	if err != nil {
		return nil, err
	}
	var submissions []domain.Submission
	err = attributevalue.UnmarshalListOfMaps(out.Items, &submissions)
	if err != nil {
		return nil, err
	}
	return submissions, nil
}

func (r *SubmissionDynamoRepository) GetSubmissionsByContest(contestID string) ([]domain.Submission, error) {
	contestIDVal, _ := attributevalue.Marshal(contestID)
	out, err := r.db.Scan(context.TODO(), &dynamodb.ScanInput{
		TableName:        &r.tableName,
		FilterExpression: aws.String("contest_id = :contest_id"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":contest_id": contestIDVal,
		},
	})
	if err != nil {
		return nil, err
	}
	var submissions []domain.Submission
	err = attributevalue.UnmarshalListOfMaps(out.Items, &submissions)
	if err != nil {
		return nil, err
	}
	return submissions, nil
}

func (r *SubmissionDynamoRepository) GetSubmissionsByStudent(studentID string) ([]domain.Submission, error) {
	studentIDVal, err := attributevalue.Marshal(studentID)
	if err != nil {
		return nil, err
	}

	out, err := r.db.Query(context.TODO(), &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		IndexName:              aws.String("submission_index"),
		KeyConditionExpression: aws.String("id = :sid"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":sid": studentIDVal,
		},
	})
	if err != nil {
		return nil, err
	}

	var submissions []domain.Submission
	err = attributevalue.UnmarshalListOfMaps(out.Items, &submissions)
	if err != nil {
		return nil, err
	}

	return submissions, nil
}

