package domain

import "github.com/golang-jwt/jwt/v5"

type Admin struct {
    ID         string `json:"id"          dynamodbav:"id"`
    Email      string `json:"email"       dynamodbav:"email"`
    IsApproved bool   `json:"is_approved" dynamodbav:"is_approved"`
    Name       string `json:"name"        dynamodbav:"name"`
    Password   string `json:"password"    dynamodbav:"password"`
}
type CustomClaims struct {
	UserID string `json:"user_id"`
	jwt.RegisteredClaims
}