# Feedback System Migration Guide

This document outlines the complete migration of the feedback system from Python/Firebase to Go/DynamoDB with Clean Architecture.

## Overview

The feedback system consists of three main components:
1. **Feedback Questions** - Created by admins for students to answer
2. **Poll Options** - Score ranges that determine if contact information is required
3. **Feedback Responses** - Student submissions with their answers and contact info

## Architecture

### Clean Architecture Layers

1. **Domain Layer** (`internal/domain/feedback.go`)
   - Defines the core business entities
   - Contains no external dependencies

2. **Repository Layer** (`internal/repository/`)
   - `feedback_question_dynamo.go`
   - `poll_option_dynamo.go`
   - `feedback_response_dynamo.go`
   - Handles data persistence with DynamoDB

3. **Use Case Layer** (`internal/usecase/feedback_usecase.go`)
   - Contains business logic
   - Orchestrates between repositories and handlers

4. **Handler Layer** (`internal/handler/http/feedback_handler.go`)
   - HTTP request/response handling
   - Input validation and output formatting

## Database Schema

### DynamoDB Tables

#### 1. feedback_questions
```json
{
  "id": "string (UUID)",
  "admin_id": "string",
  "question": "string",
  "options": ["string"],
  "is_active": "boolean",
  "created_at": "timestamp",
  "updated_at": "timestamp"
}
```

#### 2. poll_options
```json
{
  "id": "string (UUID)",
  "label": "string",
  "min_score": "number",
  "max_score": "number",
  "requires_contact": "boolean",
  "created_at": "timestamp",
  "updated_at": "timestamp"
}
```

#### 3. feedback_responses
```json
{
  "id": "string (UUID)",
  "student_id": "string",
  "student_name": "string",
  "score": "number",
  "comment": "string",
  "poll_response": "string",
  "contact_info": {
    "phone_number": "string",
    "language": "string"
  },
  "question_responses": {
    "question_id": "selected_option"
  },
  "submitted_at": "timestamp"
}
```

## API Endpoints

### Feedback Questions
- `GET /api/feedback-question/` - Get all questions
- `GET /api/feedback-question/active` - Get active questions only
- `GET /api/feedback-question/:id` - Get question by ID
- `POST /api/feedback-question/` - Create new question
- `PUT /api/feedback-question/:id` - Update question
- `DELETE /api/feedback-question/:id` - Delete question
- `GET /api/feedback-question/admin/:admin_id` - Get questions by admin

### Poll Options
- `GET /api/poll-option/` - Get all poll options
- `GET /api/poll-option/:id` - Get poll option by ID
- `GET /api/poll-option/score/:score` - Get poll option by score
- `POST /api/poll-option/` - Create new poll option
- `PUT /api/poll-option/:id` - Update poll option
- `DELETE /api/poll-option/:id` - Delete poll option

### Feedback Responses
- `GET /api/feedback-response/` - Get all responses
- `GET /api/feedback-response/:id` - Get response by ID
- `POST /api/feedback-response/` - Submit new response
- `PUT /api/feedback-response/:id` - Update response
- `DELETE /api/feedback-response/:id` - Delete response
- `GET /api/feedback-response/student/:student_id` - Get responses by student
- `GET /api/feedback-response/question/:question_id` - Get responses by question

## Setup Instructions

### 1. Create DynamoDB Tables
```bash
cd contest-backend-go/cmd/setup-tables
go run main.go
```

### 2. Configure Environment Variables
Make sure your AWS credentials are configured:
```bash
export AWS_ACCESS_KEY_ID=your_access_key
export AWS_SECRET_ACCESS_KEY=your_secret_key
export AWS_REGION=eu-north-1
```

### 3. Run the Go Backend
```bash
cd contest-backend-go
go run main.go
```

### 4. Update Frontend Configuration
Update the API base URL in both admin panel and student app:
```typescript
// In .env files
VITE_API_URL=http://localhost:8081
```

## Frontend Changes

### Admin Panel Updates
- Updated API endpoints to match Go backend
- Modified data structure to use snake_case (Go convention)
- Added proper error handling for new endpoints

### Student App Updates
- Completely redesigned feedback form
- Dynamic questions from admin
- Poll options with score ranges
- Conditional contact information form
- Integration with Telegram user data

## Business Logic

### Poll Option Logic
- Students select a score range
- If the range requires contact (`requires_contact: true`), contact form appears
- Contact form includes phone number, score, and language preference
- Score validation ensures it's within the selected range

### Question Management
- Admins can create questions with multiple choice options
- Questions can be activated/deactivated
- Only active questions appear to students
- Questions are tied to specific admins

### Response Processing
- All question responses are stored as key-value pairs
- Contact information is optional and conditional
- Responses include student identification from Telegram
- Timestamps are automatically added

## Migration Notes

### From Firebase to DynamoDB
- Replaced Firestore collections with DynamoDB tables
- Changed from document-based to item-based storage
- Updated query patterns for DynamoDB's scan/query operations

### From Python to Go
- Implemented Clean Architecture pattern
- Added proper error handling and validation
- Used Go's strong typing for better data safety
- Implemented repository pattern for data access

### API Changes
- Changed from RESTful endpoints to Go-style routing
- Updated response formats to match Go conventions
- Added proper HTTP status codes and error messages

## Testing

### Manual Testing Checklist
1. Create feedback questions as admin
2. Activate/deactivate questions
3. Create poll options with different score ranges
4. Submit feedback as student
5. Verify contact form appears for high scores
6. Check response storage in DynamoDB
7. View responses in admin panel

### API Testing
Use tools like Postman or curl to test endpoints:
```bash
# Test creating a question
curl -X POST http://localhost:8081/api/feedback-question/ \
  -H "Content-Type: application/json" \
  -d '{"question":"How was your experience?","options":["Good","Bad"],"admin_id":"admin1","is_active":true}'
```

## Troubleshooting

### Common Issues
1. **DynamoDB Connection**: Ensure AWS credentials are properly configured
2. **CORS Issues**: Check that frontend URLs are in the CORS allowlist
3. **Missing Tables**: Run the setup script to create required tables
4. **API Endpoints**: Verify the Go server is running on port 8081

### Debug Mode
Enable debug logging by setting the log level in the Go application.

## Future Enhancements

1. **Analytics Dashboard**: Add response analytics and charts
2. **Email Notifications**: Notify admins of new high-score responses
3. **Bulk Operations**: Add bulk import/export for questions
4. **Advanced Filtering**: Add filters for responses by date, score, etc.
5. **Multi-language Support**: Expand language options beyond English/Amharic 