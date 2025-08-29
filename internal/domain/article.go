package domain

import "time"

type ArticleStatus string

const (
    ArticleStatusDraft     ArticleStatus = "draft"
    ArticleStatusPublished ArticleStatus = "published"
    ArticleStatusArchived  ArticleStatus = "archived"
)

type Author struct {
    ID     string `dynamodbav:"id" json:"id"`
    Name   string `dynamodbav:"name" json:"name"`
    Avatar string `dynamodbav:"avatar" json:"avatar,omitempty"`
}

type Article struct {
	ID          string         `json:"id" dynamodbav:"id"`
	Title       string         `json:"title" dynamodbav:"title"`
	Content     string         `json:"content" dynamodbav:"content"`
	Excerpt     string         `json:"excerpt" dynamodbav:"excerpt"`
	Tags        []string       `json:"tags" dynamodbav:"tags"`
	Status      ArticleStatus  `json:"status" dynamodbav:"status"`
	Author      Author         `json:"author" dynamodbav:"author"`
	PublishedAt *time.Time     `json:"publishedAt,omitempty" dynamodbav:"publishedAt,omitempty"`
	CreatedAt   time.Time      `json:"createdAt" dynamodbav:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt" dynamodbav:"updatedAt"`
	Thumbnail   string         `json:"thumbnail,omitempty" dynamodbav:"thumbnail,omitempty"`
	ReadTime    int            `json:"readTime" dynamodbav:"readTime"`
	ViewCount   int            `json:"viewCount" dynamodbav:"viewCount"`
	LikeCount   int            `json:"likeCount" dynamodbav:"likeCount"`
	CommentCount   int            `json:"commentCount" dynamodbav:"commentCount"`
}
type Comment struct {
	ID        string    `json:"id" dynamodbav:"id"`
	ArticleID string    `json:"articleId" dynamodbav:"articleId"`
	UserId    string	`json:"user_id" dynamodbav:"user_id"`
	UserName  string	`json:"user_name" dynamodbav:"user_name"`
	Avatar    string    `json:"avatar" dynamodbav:"avatar"`
	Text   	  string    `json:"text" dynamodbav:"text"`
	CreatedAt time.Time `json:"createdAt" dynamodbav:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt" dynamodbav:"updatedAt"`
}

