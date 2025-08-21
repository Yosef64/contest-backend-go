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

type StudentDynamoRepository struct {
	db        *dynamodb.Client
	tableName string
}

func NewStudentDynamoRepository(region string, tablename string) *StudentDynamoRepository {
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region),
	)
	if err != nil {
		panic("unable to load AWS SDK config: " + err.Error())
	}
	return &StudentDynamoRepository{
		db:        dynamodb.NewFromConfig(cfg),
		tableName: tablename,
	}
}

func (r *StudentDynamoRepository) AddStudent(student domain.Student) error {
	if student.ID == "" {
		student.ID = uuid.New().String()
	}

	if student.TelegramID == "" {
		return fmt.Errorf("telegram_id is required and cannot be empty")
	}

	// Set CreatedAt timestamp if not already set
	if student.CreatedAt.IsZero() {
		student.CreatedAt = time.Now()
	}

	item, err := attributevalue.MarshalMap(student)
	if err != nil {
		return err
	}

	_, err = r.db.PutItem(context.TODO(), &dynamodb.PutItemInput{
		TableName: &r.tableName,
		Item:      item,
	})
	return err
}

func (r *StudentDynamoRepository) UpdateStudent(student domain.Student) error {
	item, err := attributevalue.MarshalMap(student)
	if err != nil {
		return err
	}
	_, err = r.db.PutItem(context.TODO(), &dynamodb.PutItemInput{
		TableName: &r.tableName,
		Item:      item,
	})
	return err
}

func (r *StudentDynamoRepository) GetStudentByID(id string) (*domain.Student, error) {

	out, err := r.db.Query(context.TODO(), &dynamodb.QueryInput{
		TableName:              &r.tableName,
		KeyConditionExpression: aws.String("id = :id"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":id": &types.AttributeValueMemberS{Value: id},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(out.Items) == 0 {
		return nil, nil // Student not found
	}

	var student domain.Student
	err = attributevalue.UnmarshalMap(out.Items[0], &student)
	if err != nil {
		return nil, err
	}
	return &student, nil
}

func (r *StudentDynamoRepository) GetStudentByTelegramID(telegramID string) (*domain.Student, error) {
	teleIDVal, _ := attributevalue.Marshal(telegramID)
	out, err := r.db.Scan(context.TODO(), &dynamodb.ScanInput{
		TableName:        &r.tableName,
		FilterExpression: aws.String("telegram_id = :tele_id"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":tele_id": teleIDVal,
		},
	})
	if err != nil {
		return nil, err
	}
	if len(out.Items) == 0 {
		return nil, nil // Student not found
	}

	var student domain.Student
	err = attributevalue.UnmarshalMap(out.Items[0], &student)
	if err != nil {
		return nil, err
	}
	return &student, nil
}

func (r *StudentDynamoRepository) GetStudents() ([]domain.Student, error) {
	out, err := r.db.Scan(context.TODO(), &dynamodb.ScanInput{
		TableName: &r.tableName,
	})
	if err != nil {
		return nil, err
	}
	var students []domain.Student
	err = attributevalue.UnmarshalListOfMaps(out.Items, &students)
	if err != nil {
		return nil, err
	}
	return students, nil
}
func (r *StudentDynamoRepository) GetStructuredStudents() (map[string]domain.Student, error) {
	structured := make(map[string]domain.Student)
	students, err := r.GetStudents()
	if err != nil {
		return nil, err
	}
	for _, stud := range students {
		structured[stud.ID] = stud
	}
	return structured, nil
}

func (r *StudentDynamoRepository) VerifyStudentPaid(telegramID string) (bool, error) {
	teleIDVal, _ := attributevalue.Marshal(telegramID)
	paidVal, _ := attributevalue.Marshal(true)
	out, err := r.db.Query(context.TODO(), &dynamodb.QueryInput{
		TableName:        &r.tableName,
		FilterExpression: aws.String("telegram_id = :tele_id AND paid = :paid"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":tele_id": teleIDVal,
			":paid":    paidVal,
		},
	})
	if err != nil {
		return false, err
	}
	return len(out.Items) > 0, nil
}

func (r *StudentDynamoRepository) GetPaidStudents() ([]domain.Student, error) {
	paidVal, _ := attributevalue.Marshal(true)
	out, err := r.db.Query(context.TODO(), &dynamodb.QueryInput{
		TableName:        &r.tableName,
		FilterExpression: aws.String("paid = :paid"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":paid": paidVal,
		},
	})
	if err != nil {
		return nil, err
	}
	var students []domain.Student
	err = attributevalue.UnmarshalListOfMaps(out.Items, &students)
	if err != nil {
		return nil, err
	}
	return students, nil
}

func (r *StudentDynamoRepository) GetGradesAndSchools() (map[string][]string, error) {
	out, err := r.db.Scan(context.TODO(), &dynamodb.ScanInput{
		TableName:            &r.tableName,
		ProjectionExpression: aws.String("grade, school"),
	})
	if err != nil {
		return nil, err
	}
	gradesSet := make(map[string]struct{})
	schoolsSet := make(map[string]struct{})
	for _, item := range out.Items {
		var student domain.Student
		err := attributevalue.UnmarshalMap(item, &student)
		if err != nil {
			continue
		}
		if student.Grade != "" {
			gradesSet[student.Grade] = struct{}{}
		}
		if student.School != "" {
			schoolsSet[student.School] = struct{}{}
		}
	}
	grades := make([]string, 0, len(gradesSet))
	schools := make([]string, 0, len(schoolsSet))
	for g := range gradesSet {
		grades = append(grades, g)
	}
	for s := range schoolsSet {
		schools = append(schools, s)
	}
	return map[string][]string{"grades": grades, "schools": schools}, nil
}

func (r *StudentDynamoRepository) GetUserProfile(studentID string) (map[string]interface{}, error) {
	student, err := r.GetStudentByID(studentID)
	if err != nil || student == nil {
		return nil, err
	}
	profile := map[string]interface{}{
		"id":          student.ID,
		"telegram_id": student.TelegramID,
		"name":        student.Name,
		"age":         student.Age,
		"grade":       student.Grade,
		"school":      student.School,
		"city":        student.City,
		"region":      student.Region,
		"imgurl":      student.ImgURL,
		"isSuspended": student.IsSuspended,
		"badge":       student.Badge,
		"is_premium":  student.IsPremium,
	}
	return profile, nil
}

func (r *StudentDynamoRepository) GetQuickStat(studentID string) (map[string]interface{}, error) {
	student, err := r.GetStudentByID(studentID)
	if err != nil || student == nil {
		return nil, err
	}
	// Placeholder: In a real implementation, aggregate stats from submissions, etc.
	return map[string]interface{}{
		"telegram_id":        student.TelegramID,
		"name":               student.Name,
		"totalPoints":        0,   // TODO: Calculate from submissions
		"payment":            nil, // TODO: Integrate with payment table
		"contestSubmissions": nil, // TODO: Integrate with submissions
	}, nil
}

func (r *StudentDynamoRepository) GetStudentRankings() ([]map[string]interface{}, error) {
	// TODO: Implement aggregation logic across students and submissions
	return nil, nil
}

func (r *StudentDynamoRepository) GetStudentRankingsByContest(contestID string) ([]map[string]interface{}, error) {
	// TODO: Implement aggregation logic for rankings by contest
	return nil, nil
}

func (r *StudentDynamoRepository) DeleteStudent(id string) error {
	_, err := r.db.DeleteItem(context.TODO(), &dynamodb.DeleteItemInput{
		TableName: &r.tableName,
		Key: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: id},
		},
	})
	return err
}
