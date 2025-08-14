package repository

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"victor-contest-go/internal/domain"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
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
	
	fmt.Printf("Raw DynamoDB item for contest %s: %+v\n", id, out.Item)
	
	var contest domain.Contest
	err = attributevalue.UnmarshalMap(out.Item, &contest)
	if err != nil {
		return nil, err
	}
	
	fmt.Printf("Unmarshaled contest: %+v\n", contest)
	fmt.Printf("Contest questions: %+v\n", contest.Questions)
	
	return &contest, nil
}

func (r *ContestDynamoRepository) UpdateContest(id string, update domain.Contest) error {
	// First, get the current contest to preserve existing fields
	currentContest, err := r.GetContestByID(id)
	if err != nil {
		return err
	}
	if currentContest == nil {
		return fmt.Errorf("contest not found")
	}

	// Use UpdateItem for more efficient partial updates
	updateExpression := "SET "
	var expressionAttributeNames map[string]string
	var expressionAttributeValues map[string]types.AttributeValue
	
	// Build update expression dynamically based on provided fields
	var updateParts []string
	expressionAttributeNames = make(map[string]string)
	expressionAttributeValues = make(map[string]types.AttributeValue)
	
	// Helper function to add field to update
	addFieldToUpdate := func(fieldName, jsonName string) {
		if fieldValue := reflect.ValueOf(update).FieldByName(fieldName).String(); fieldValue != "" {
			updateParts = append(updateParts, "#"+jsonName+" = :"+jsonName)
			expressionAttributeNames["#"+jsonName] = jsonName
			expressionAttributeValues[":"+jsonName] = &types.AttributeValueMemberS{Value: fieldValue}
		}
	}
	
	// Add each field if it has a value
	addFieldToUpdate("Title", "title")
	addFieldToUpdate("Description", "description")
	addFieldToUpdate("StartTime", "start_time")
	addFieldToUpdate("EndTime", "end_time")
	addFieldToUpdate("Subject", "subject")
	addFieldToUpdate("Grade", "grade")
	addFieldToUpdate("Prize", "prize")
	addFieldToUpdate("Status", "status")
	addFieldToUpdate("Type", "type")
	
	// ALWAYS preserve the questions field - this is critical!
	// Even if no other fields are being updated, we must preserve questions
	fmt.Printf("Current contest questions before update: %+v\n", currentContest.Questions)
	
	// Always add questions to the update expression
	updateParts = append(updateParts, "#questions = :questions")
	expressionAttributeNames["#questions"] = "questions"
	
	// Handle both cases: when questions exist and when they don't
	if len(currentContest.Questions) > 0 {
		fmt.Printf("Preserving existing questions: %+v\n", currentContest.Questions)
		expressionAttributeValues[":questions"] = &types.AttributeValueMemberSS{Value: currentContest.Questions}
	} else {
		fmt.Printf("Setting empty questions array for contest %s\n", id)
		// Set an empty string set to preserve the field structure
		expressionAttributeValues[":questions"] = &types.AttributeValueMemberSS{Value: []string{}}
	}
	
	// Build the final update expression
	updateExpression += strings.Join(updateParts, ", ")
	
	fmt.Printf("Final update expression: %s\n", updateExpression)
	fmt.Printf("Expression attribute names: %+v\n", expressionAttributeNames)
	fmt.Printf("Expression attribute values: %+v\n", expressionAttributeValues)
	
	// Create the key for the item to update
	key, err := attributevalue.MarshalMap(map[string]string{"id": id})
	if err != nil {
		return err
	}
	
	// Perform the update
	_, err = r.db.UpdateItem(context.TODO(), &dynamodb.UpdateItemInput{
		TableName:                 &r.tableName,
		Key:                       key,
		UpdateExpression:          &updateExpression,
		ExpressionAttributeNames:  expressionAttributeNames,
		ExpressionAttributeValues: expressionAttributeValues,
	})
	
	if err != nil {
		fmt.Printf("UpdateItem error: %v\n", err)
	} else {
		fmt.Printf("UpdateItem successful for contest %s\n", id)
	}
	
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