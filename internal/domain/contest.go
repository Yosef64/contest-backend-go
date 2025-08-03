package domain


type Contest struct {
    ID               string   `json:"id"                 dynamodbav:"id"`
    Title            string   `json:"title"              dynamodbav:"title"`
    Description      string   `json:"description"        dynamodbav:"description"`
    StartTime        string   `json:"start_time"         dynamodbav:"start_time"`
    EndTime          string   `json:"end_time"           dynamodbav:"end_time"`
    Subject          string   `json:"subject"            dynamodbav:"subject"`
    Grade            string   `json:"grade"              dynamodbav:"grade"`
    Prize            string   `json:"prize"              dynamodbav:"prize"`
    Questions        []string `json:"questions"          dynamodbav:"questions"`
    Status           string `json:"status"               dynamodbav:"status"`
    Type              string `json:"type"  dynamodbav:"type"`
}

type ContestAnnouncementRequest struct{
    Message string `json:"message"`
}
type ContestTypeWithQuestionObj struct{
    Contest
    Questions []Question `json:"questions"`
}