package domain

type Question struct {
	ID               string   `json:"id"                 dynamodbav:"id"`
	QuestionText     string   `json:"question_text"      dynamodbav:"question_text"`
	Answer           int      `json:"answer"             dynamodbav:"answer"`
	QuestionImg      string   `json:"question_image"     dynamodbav:"question_img"`
	Explanation      string   `json:"explanation"        dynamodbav:"explanation"`
	ExplanationImg   string   `json:"explanation_image"  dynamodbav:"explanation_image"`
	Subject          string   `json:"subject"            dynamodbav:"subject"`
	Grade            string   `json:"grade"              dynamodbav:"grade"`
	Chapter          string   `json:"chapter"            dynamodbav:"chapter"`
	MultipleChoice   []string `json:"multiple_choice"    dynamodbav:"multiple_choice"`
}

type AiPracticeSetting struct {
	Subject    string `json:"subject"`
	Topic      string `json:"topic"`
	Difficulty string `json:"difficulty"`
}
type MultipleQuestionRequest struct {
	Questions []Question `json:"questions"`
}
