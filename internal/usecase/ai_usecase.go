package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"victor-contest-go/internal/domain"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

type aiUsecase struct {
	subRepo SubmissionRepository
}

func (a *aiUsecase) GenerateRecommendations(input domain.RecommendationInput) (*domain.Recommendations, error) {
	inputJSON, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal input data: %w", err)
	}

	// 2. Craft a detailed prompt for the Gemini API.
	prompt := fmt.Sprintf(`
	You are an expert academic tutor creating a study plan for a mobile app.
	Your response MUST be optimized for a mobile view: be concise, structured, and easy to read.

	**Student Performance Data:**
	%s

	**Instructions:**
	1.  Identify the relevant chapters for the subject: "%s". Ignore irrelevant ones.
	2.  Analyze the student's weakest relevant topics.
	3.  For each resource, you MUST specify the 'platform' where it is located (e.g., "Website", "YouTube", "Book").
	4.  Generate a response using the exact JSON structure below.
	5.  Keep all strings short and actionable.
	6.  Break the practice plan into weekly or daily steps, not a long paragraph.
	7.  **Your entire output must be a single, valid JSON object and nothing else.**

	**Required JSON Output Structure:**
	{
	"recommendations": ["short phrase 1", "short phrase 2"],
	"strategies": ["short phrase 1", "short phrase 2"],
	"resources": [
		{"name": "Resource Title", "topic": "Specific Topic", "platform": "Website/YouTube/Book", "type": "Video/Practice"}
	],
	"practicePlan": [
		{"timeframe": "Week 1", "focus": "A short description of the focus."}
	]
	}
	`, string(inputJSON), input.Subject)
	rawText,err := generateContentFromAPI(prompt)

	startIndex := strings.Index(rawText, "{")
	endIndex := strings.LastIndex(rawText, "}")

	// Check if a valid JSON object was found.
	if startIndex == -1 || endIndex == -1 {
		log.Printf("Raw response from API: %s", rawText)
		return nil, errors.New("could not find JSON object in the API response")
	}

	// Slice the string to get only the JSON part.
	jsonStr := rawText[startIndex : endIndex+1]
	if err != nil {
		return nil, err
	}
	var recommendations domain.Recommendations
	if err := json.Unmarshal([]byte(jsonStr), &recommendations); err != nil {
		log.Printf("Raw response from API: %s", jsonStr)
		return nil, fmt.Errorf("failed to unmarshal JSON response from API: %w", err)
	}

	log.Println("✅ Successfully generated and parsed recommendations!")
	return &recommendations, nil
}

// PracticeWithAi implements AiUsecase.
func (a *aiUsecase) PracticeWithAi(setting domain.AiPracticeSetting) (*[]domain.Question, error) {
	// 1. Construct the prompt using Go's standard library.
	prompt := fmt.Sprintf(`
Generate 25 practice questions for a student.
The topic is %s for grade %s at a %s difficulty level.

For each question, provide the following in a clear, structured format:
- The question text.
- Four multiple-choice options.
- The index of the correct answer (0, 1, 2, or 3).
- A brief explanation for the correct answer.

IMPORTANT: Format the entire output as a JSON array where each element is a question object.
Each object must have these exact keys: "question_text", "multiple_choice", "answer", "explanation", "subject", "grade", "chapter".
`, setting.Subject, setting.Topic, setting.Difficulty)

	// 2. Call the API to get the raw text response.
	rawText, err := generateContentFromAPI(prompt)
	if err != nil {
		log.Printf("Error calling generation API: %v", err)
		return nil, fmt.Errorf("API call failed: %w", err)
	}

	// 3. Reliably extract the JSON array from the raw text.
	startIndex := strings.Index(rawText, "[")
	endIndex := strings.LastIndex(rawText, "]")

	if startIndex == -1 || endIndex == -1 || startIndex > endIndex {
		msg := "could not find a valid JSON array in the API response"
		log.Printf("%s. Raw text: %s", msg, rawText)
		return nil, errors.New(msg)
	}

	jsonStr := rawText[startIndex : endIndex+1]

	var questions []domain.Question
	if err := json.Unmarshal([]byte(jsonStr), &questions); err != nil {
		log.Printf("Error unmarshalling JSON: %v. JSON string: %s", err, jsonStr)
		return nil, fmt.Errorf("failed to parse JSON from API: %w", err)
	}

	return &questions, nil

}
func generateContentFromAPI(prompt string) (string, error) {
	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		return "", errors.New("GOOGLE_API_KEY environment variable not set")
	}

	ctx := context.Background()

	// 1. Create a new Gemini client.
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return "", fmt.Errorf("error creating gemini client: %w", err)
	}
	defer client.Close()

	// 2. Select the model.
	model := client.GenerativeModel("gemini-2.5-flash")

	// 3. Generate the content. The SDK handles all the HTTP and JSON work.
	log.Println("🤖 Calling Gemini API via Go SDK to generate session...")
	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return "", fmt.Errorf("error generating content: %w", err)
	}
	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", errors.New("no content found in gemini response")
	}

	var responseText strings.Builder
	for _, part := range resp.Candidates[0].Content.Parts {
		if txt, ok := part.(genai.Text); ok {
			responseText.WriteString(string(txt))
		}
	}

	log.Println("✅ Successfully received response from Gemini.")
	return responseText.String(), nil
}

type AiUsecase interface {
	PracticeWithAi(seting domain.AiPracticeSetting) (*[]domain.Question, error)
	GenerateRecommendations(input domain.RecommendationInput) (*domain.Recommendations, error)
}

func NewAiUsecase(subRepo SubmissionRepository) AiUsecase {
	return &aiUsecase{subRepo: subRepo}
}
