package domain

import "time"

type ContestRegistration struct {
    ID           string `json:"id"            dynamodbav:"id"`
    ContestID    string `json:"contest_id"    dynamodbav:"contest_id"`
    StudentID    string `json:"student_id"    dynamodbav:"student_id"`
    IsActive     bool   `json:"is_active"     dynamodbav:"is_active"`
    RegisteredAt time.Time `json:"registered_at" dynamodbav:"registered_at"`
}

type ContestRegistrationDto struct{
    ContestID    string `json:"contest_id"    dynamodbav:"contest_id"`
    StudentID    string `json:"student_id"    dynamodbav:"student_id"`
}