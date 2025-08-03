package repository

import (
	"context"
	"victor-contest-go/internal/domain"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/google/uuid"
)

type ContestDynamoRepository struct {
	db        *dynamodb.Client
	tableName string
}

func NewContestDynamoRepository(region string,tablename string) *ContestDynamoRepository {
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region),
	)
	if err != nil {
		panic("unable to load AWS SDK config: " + err.Error())
	}
	return &ContestDynamoRepository{
		db:        dynamodb.NewFromConfig(cfg),
		tableName: tablename,
	}
}

func (r *ContestDynamoRepository) AddContest(contest domain.Contest) (string, error) {
	if contest.ID == "" {
		contest.ID = uuid.New().String()
	}
	item, err := attributevalue.MarshalMap(contest)
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
	return contest.ID, nil
}

func (r *ContestDynamoRepository) GetAllContests() ([]domain.Contest, error) {
	out, err := r.db.Scan(context.TODO(), &dynamodb.ScanInput{
		TableName: &r.tableName,
	})
	if err != nil {
		return nil, err
	}
	var contests []domain.Contest
	err = attributevalue.UnmarshalListOfMaps(out.Items, &contests)
	if err != nil {
		return nil, err
	}
	return contests, nil
}

func (r *ContestDynamoRepository) GetContestByID(id string) (*domain.Contest, error) {
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
		return nil, nil // Not found
	}
	var contest domain.Contest
	err = attributevalue.UnmarshalMap(out.Item, &contest)
	if err != nil {
		return nil, err
	}
	return &contest, nil
}

func (r *ContestDynamoRepository) UpdateContest(id string, update domain.Contest) error {
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

func (r *ContestDynamoRepository) DeleteContest(id string) error {
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