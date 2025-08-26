package usecase

import (
	"fmt"
	"time"
	"victor-contest-go/internal/domain"
)

type ArticleUsecase struct {
    repo ArticleRepository
    commentRepo CommentRepository
}

func NewArticleUsecase(repo ArticleRepository, commentRepo CommentRepository) *ArticleUsecase {
    return &ArticleUsecase{repo: repo, commentRepo: commentRepo}
}

func (u *ArticleUsecase) Create(input domain.Article) (string, error) {
    if input.Status == "" { input.Status = domain.ArticleStatusDraft }
    now := time.Now()
    if input.CreatedAt.IsZero() { input.CreatedAt = now }
    input.UpdatedAt = now
    if input.Status == domain.ArticleStatusPublished {
        input.PublishedAt = &now
    } else {
        input.PublishedAt = nil
    }
    input.CreatedAt = now
    return u.repo.Create(input)
}

func (u *ArticleUsecase) Update(id string, update domain.Article) error {
    update.UpdatedAt = time.Now()
    return u.repo.Update(id, update)
}

func (u *ArticleUsecase) Delete(id string) error { return u.repo.Delete(id) }

func (u *ArticleUsecase) GetByID(id string) (*domain.Article, error) { return u.repo.GetByID(id) }

func (u *ArticleUsecase) List() ([]domain.Article, error) { return u.repo.List() }

func (u *ArticleUsecase) ListPublished() ([]domain.Article, error) { return u.repo.ListPublished() }

func (u *ArticleUsecase) GetByStatus(status domain.ArticleStatus) ([]domain.Article, error) { return u.repo.GetByStatus(status) }

func (uc *ArticleUsecase) ToggleStatus(id string, status domain.ArticleStatus) error {
	article, err := uc.repo.GetByID(id)
	if err != nil {
		return err
	}
	if article == nil {
		return fmt.Errorf("article not found")
	}

	article.Status = status
	article.UpdatedAt = time.Now()
	if status == domain.ArticleStatusPublished && article.PublishedAt == nil {
		now := time.Now()
		article.PublishedAt = &now
	}

	return uc.repo.Update(id, *article)
}

func (uc *ArticleUsecase) IncrementView(id string) error {
	return uc.repo.IncrementView(id)
}

func (uc *ArticleUsecase) IncrementLike(id string) error {
	return uc.repo.IncrementLike(id)
}

func (uc *ArticleUsecase) DecrementView(id string) error {
	return uc.repo.DecrementView(id)
}

func (uc *ArticleUsecase) DecrementLike(id string) error {
	return uc.repo.DecrementLike(id)
}

// Comment methods
func (uc *ArticleUsecase) CreateComment(comment domain.Comment) (string, error) {
	return uc.commentRepo.Create(comment)
}

func (uc *ArticleUsecase) ListCommentsByArticleID(articleID string) ([]domain.Comment, error) {
	return uc.commentRepo.ListByArticleID(articleID)
}


