package repository

import (
	"context"
	"errors"
	"fmt"
	"victor-contest-go/internal/domain"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
)

type ContestRegistrationDynamoRepository struct {
	db        *dynamodb.Client
	tableName string
}

func NewContestRegistrationDynamoRepository(region string , tablename string) *ContestRegistrationDynamoRepository {
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
		registration.ID = uuid.New().String()
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
	compositeID := fmt.Sprintf("%s#%s", contestID, user_id)

    out, err := r.db.Query(context.TODO(), &dynamodb.QueryInput{
        TableName: &r.tableName,
        KeyConditionExpression: aws.String("id = :id"),
        ExpressionAttributeValues: map[string]types.AttributeValue{
            ":id": &types.AttributeValueMemberS{Value: compositeID},
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

