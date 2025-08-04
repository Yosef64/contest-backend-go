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

type PollOptionDynamoRepository struct {
	db        *dynamodb.Client
	tableName string
}

func NewPollOptionDynamoRepository(region string, tablename string) *PollOptionDynamoRepository {
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region),
	)
	if err != nil {
		panic("unable to load AWS SDK config: " + err.Error())
	}
	return &PollOptionDynamoRepository{
		db:        dynamodb.NewFromConfig(cfg),
		tableName: tablename,
	}
}

func (r *PollOptionDynamoRepository) AddPollOption(option domain.PollOption) (string, error) {
	if option.ID == "" {
		option.ID = uuid.New().String()
	}
	item, err := attributevalue.MarshalMap(option)
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
	return option.ID, nil
}

func (r *PollOptionDynamoRepository) UpdatePollOption(id string, update domain.PollOption) error {
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

func (r *PollOptionDynamoRepository) DeletePollOption(id string) error {
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

func (r *PollOptionDynamoRepository) GetPollOptionByID(id string) (*domain.PollOption, error) {
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
	var option domain.PollOption
	err = attributevalue.UnmarshalMap(out.Item, &option)
	if err != nil {
		return nil, err
	}
	return &option, nil
}

func (r *PollOptionDynamoRepository) GetAllPollOptions() ([]domain.PollOption, error) {
	out, err := r.db.Scan(context.TODO(), &dynamodb.ScanInput{
		TableName: &r.tableName,
	})
	if err != nil {
		return nil, err
	}
	var options []domain.PollOption
	err = attributevalue.UnmarshalListOfMaps(out.Items, &options)
	if err != nil {
		return nil, err
	}
	return options, nil
}

func (r *PollOptionDynamoRepository) GetPollOptionByScore(score int) (*domain.PollOption, error) {
	scoreVal, _ := attributevalue.Marshal(score)
	out, err := r.db.Scan(context.TODO(), &dynamodb.ScanInput{
		TableName: &r.tableName,
		FilterExpression: aws.String("min_score <= :score AND max_score >= :score"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":score": scoreVal,
		},
	})
	if err != nil {
		return nil, err
	}
	if len(out.Items) == 0 {
		return nil, nil
	}
	var option domain.PollOption
	err = attributevalue.UnmarshalMap(out.Items[0], &option)
	if err != nil {
		return nil, err
	}
	return &option, nil
} 