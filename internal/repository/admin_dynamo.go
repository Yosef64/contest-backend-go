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

type AdminDynamoRepository struct {
	db        *dynamodb.Client
	tableName string
}

func NewAdminDynamoRepository(region,tableName string) *AdminDynamoRepository {
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region),
	)
	if err != nil {
		panic("unable to load AWS SDK config: " + err.Error())
	}
	return &AdminDynamoRepository{
		db:        dynamodb.NewFromConfig(cfg),
		tableName: tableName,
	}
}

func (r *AdminDynamoRepository) AddAdmin(admin domain.Admin) (string, error) {
	if admin.ID == "" {
		admin.ID = uuid.New().String()
	}
	item, err := attributevalue.MarshalMap(admin)
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
	return admin.ID, nil
}

func (r *AdminDynamoRepository) UpdateAdmin(id string, update domain.Admin) error {
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

func (r *AdminDynamoRepository) DeleteAdmin(id string) error {
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

func (r *AdminDynamoRepository) GetAdminByID(id string) (*domain.Admin, error) {
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
	var admin domain.Admin
	err = attributevalue.UnmarshalMap(out.Item, &admin)
	if err != nil {
		return nil, err
	}
	return &admin, nil
}
func (r *AdminDynamoRepository) GetAdminByEmail(email string) (*domain.Admin,error) {
	key,err := attributevalue.Marshal(email)
	if err != nil {
		return nil, err
	}
	out,err := r.db.Query(context.TODO(),&dynamodb.QueryInput{
		IndexName: aws.String("email-id-index"),
		TableName: &r.tableName,
		KeyConditionExpression: aws.String("email = :email"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":email":key,
		},

	})
	if err != nil {
		return nil, err
	}
	if out.Items ==nil {
		return nil,nil
	}
	var admin domain.Admin
	err = attributevalue.UnmarshalMap(out.Items[0],&admin)
	if err != nil {
		return nil, err
	}
	return &admin,nil
	
}
func (r *AdminDynamoRepository) GetAllAdmins() ([]domain.Admin, error) {
	out, err := r.db.Scan(context.TODO(), &dynamodb.ScanInput{
		TableName: &r.tableName,
	})
	if err != nil {
		return nil, err
	}
	var admins []domain.Admin
	err = attributevalue.UnmarshalListOfMaps(out.Items, &admins)
	if err != nil {
		return nil, err
	}
	return admins, nil
}

func (r *AdminDynamoRepository) SignIn(email, password string) (*domain.Admin, error) {
    emailVal, _ := attributevalue.Marshal(email)

    out, err := r.db.Query(context.TODO(), &dynamodb.QueryInput{
        TableName:              &r.tableName,
        IndexName:              aws.String("email-id-index"),
        KeyConditionExpression: aws.String("email = :email"),
        ExpressionAttributeValues: map[string]types.AttributeValue{
            ":email": emailVal,
        },
    })
    if err != nil {
        return nil, err
    }

    if len(out.Items) == 0 {
        return nil, nil
    }

    var admin domain.Admin
    
    if err := attributevalue.UnmarshalMap(out.Items[0], &admin); err != nil {
        return nil, err
    }
    
    return &admin, nil
}