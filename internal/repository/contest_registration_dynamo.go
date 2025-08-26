package repository

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"victor-contest-go/internal/domain"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type ContestRegistrationDynamoRepository struct {
	db        *dynamodb.Client
	tableName string
}

func NewContestRegistrationDynamoRepository(region string, tablename string) *ContestRegistrationDynamoRepository {
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region),
	)
	if err != nil {
		panic("unable to load AWS SDK config: " + err.Error())
	}
	return &ContestRegistrationDynamoRepository{
		db:        dynamodb.NewFromConfig(cfg),
		tableName: tablename,
	}
}

func (r *ContestRegistrationDynamoRepository) AddContestRegistration(registration domain.ContestRegistration) (string, error) {
	if registration.ID == "" {
		numBytes := 5
		randomBytes := make([]byte, numBytes)

		_, err := rand.Read(randomBytes)
		if err != nil {
			return "", err
		}

		id := base64.RawURLEncoding.EncodeToString(randomBytes)
		registration.ID = id
	}
	item, err := attributevalue.MarshalMap(registration)
	if err != nil {
		return "", errors.New("invalid registration data")
	}
	_, err = r.db.PutItem(context.TODO(), &dynamodb.PutItemInput{
		TableName: &r.tableName,
		Item:      item,
	})
	if err != nil {
		return "", err
	}
	return registration.ID, nil
}

func (r *ContestRegistrationDynamoRepository) UpdateContestRegistration(id string, update domain.ContestRegistration) error {
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

func (r *ContestRegistrationDynamoRepository) DeleteContestRegistration(id string) error {
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

func (r *ContestRegistrationDynamoRepository) GetRegistrationsByContestAndStudent(contestID string, user_id string) (*domain.ContestRegistration, error) {
	contestId, err := attributevalue.Marshal(contestID)
	if err != nil {
		return nil, err
	}
	studentId, err := attributevalue.Marshal(user_id)
	if err != nil {
		return nil, err
	}
	out, err := r.db.Query(context.TODO(), &dynamodb.QueryInput{
		TableName:              &r.tableName,
		KeyConditionExpression: aws.String("contest_id = :contestId AND student_id = :studentId"),
		IndexName:              aws.String("contest_id-student_id-index"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":contestId": contestId,
			":studentId": studentId,
		},
	})
	if err != nil {
		return nil, err
	}

	if len(out.Items) == 0 {
		return nil, nil
	}

	var registration domain.ContestRegistration
	err = attributevalue.UnmarshalMap(out.Items[0], &registration)
	if err != nil {
		return nil, err
	}

	return &registration, nil

}
func (r *ContestRegistrationDynamoRepository) GetRegistrationsByContest(contest_id string) ([]domain.ContestRegistration, error) {
	contestId, err := attributevalue.Marshal(contest_id)
	if err != nil {
		return nil, err
	}
	out, err := r.db.Query(context.TODO(), &dynamodb.QueryInput{
		TableName:              &r.tableName,
		KeyConditionExpression: aws.String("contest_id = :contestId"),
		IndexName:              aws.String("contest_id-student_id-index"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":contestId": contestId,
		},
	})
	if err != nil {
		return nil, err
	}

	if len(out.Items) == 0 {
		return nil, nil
	}
	var registerations []domain.ContestRegistration
	err = attributevalue.UnmarshalListOfMaps(out.Items, &registerations)
	if err != nil {
		return nil, err
	}
	return registerations, nil
}
