package repository

import (
	"context"
	"fmt"
	"mime/multipart"
	"strings"

	"github.com/cloudinary/cloudinary-go/v2"
	"github.com/cloudinary/cloudinary-go/v2/api/admin"
	"github.com/cloudinary/cloudinary-go/v2/api/uploader"
)


type ImageRepository struct{
	cloudinaryurl string
}
func NewImageRepostory(cloud_url string) *ImageRepository{
	return &ImageRepository{cloudinaryurl: cloud_url}
}

func(r *ImageRepository) UploadImage(file multipart.File,folderName string)(string, error) {
	cld, err := cloudinary.New()
	if err != nil {
		return "", fmt.Errorf("failed to initialize cloudinary: %w", err)
	}

	ctx := context.Background()

	uploadResult, err := cld.Upload.Upload(ctx, file, uploader.UploadParams{
		Folder: "",
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload file: %w", err)
	}

	return uploadResult.SecureURL, nil
}

// ListImages returns a list of image URLs under a specific folder
func (r *ImageRepository) ListImages(folderName string, maxResults int) ([]string, error) {
	cld, err := cloudinary.New()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize cloudinary: %w", err)
	}

	ctx := context.Background()

	if maxResults <= 0 {
		maxResults = 100
	}

	res, err := cld.Admin.Assets(ctx, admin.AssetsParams{
		MaxResults: maxResults,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list images: %w", err)
	}

	urls := make([]string, 0, len(res.Assets))
	for _, asset := range res.Assets {
		urls = append(urls, asset.SecureURL)
	}
	return urls, nil
}

// DeleteImage deletes an image by its public ID or by URL
func (r *ImageRepository) DeleteImage(publicIDOrURL string) error {
	cld, err := cloudinary.New()
	if err != nil {
		return fmt.Errorf("failed to initialize cloudinary: %w", err)
	}

	ctx := context.Background()

	publicID := publicIDOrURL
	if strings.HasPrefix(publicIDOrURL, "http://") || strings.HasPrefix(publicIDOrURL, "https://") {
		parts := strings.Split(publicIDOrURL, "/upload/")
		if len(parts) == 2 {
			tail := parts[1]
			if strings.HasPrefix(tail, "v") {
				slash := strings.Index(tail, "/")
				if slash != -1 {
					tail = tail[slash+1:]
				}
			}
			if dot := strings.LastIndex(tail, "."); dot != -1 {
				tail = tail[:dot]
			}
			publicID = tail
		}
	}

	_, err = cld.Upload.Destroy(ctx, uploader.DestroyParams{PublicID: publicID})
	if err != nil {
		return fmt.Errorf("failed to delete image: %w", err)
	}
	return nil
}
