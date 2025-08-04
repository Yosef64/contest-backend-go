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

type NotificationDynamoRepository struct {
	db        *dynamodb.Client
	tableName string
}

func NewNotificationDynamoRepository(region string, tablename string) *NotificationDynamoRepository {
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region),
	)
	if err != nil {
		panic("unable to load AWS SDK config: " + err.Error())
	}
	return &NotificationDynamoRepository{
		db:        dynamodb.NewFromConfig(cfg),
		tableName: tablename,
	}
}

func (r *NotificationDynamoRepository) AddNotification(notification domain.Notification) (string, error) {
	if notification.ID == "" {
		notification.ID = uuid.New().String()
	}
	item, err := attributevalue.MarshalMap(notification)
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
	return notification.ID, nil
}

func (r *NotificationDynamoRepository) UpdateNotification(id string, update domain.Notification) error {
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

func (r *NotificationDynamoRepository) DeleteNotification(id string) error {
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

func (r *NotificationDynamoRepository) GetNotificationByID(id string) (*domain.Notification, error) {
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
	var notification domain.Notification
	err = attributevalue.UnmarshalMap(out.Item, &notification)
	if err != nil {
		return nil, err
	}
	return &notification, nil
}

func (r *NotificationDynamoRepository) GetAllNotifications() ([]domain.Notification, error) {
	out, err := r.db.Scan(context.TODO(), &dynamodb.ScanInput{
		TableName: &r.tableName,
	})
	if err != nil {
		return nil, err
	}
	var notifications []domain.Notification
	err = attributevalue.UnmarshalListOfMaps(out.Items, &notifications)
	if err != nil {
		return nil, err
	}
	return notifications, nil
}

func (r *NotificationDynamoRepository) GetNotificationsByRecipient(recipientID string) ([]domain.Notification, error) {
	recipientVal, _ := attributevalue.Marshal(recipientID)
	out, err := r.db.Scan(context.TODO(), &dynamodb.ScanInput{
		TableName:        &r.tableName,
		FilterExpression: aws.String("recipient_id = :recipient_id"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":recipient_id": recipientVal,
		},
	})
	if err != nil {
		return nil, err
	}
	var notifications []domain.Notification
	err = attributevalue.UnmarshalListOfMaps(out.Items, &notifications)
	if err != nil {
		return nil, err
	}
	return notifications, nil
}
