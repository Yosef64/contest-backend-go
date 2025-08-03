package repository

import (
	"context"
	"fmt"
	"mime/multipart"

	"github.com/cloudinary/cloudinary-go/v2"
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
		Folder: folderName,
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload file: %w", err)
	}

	return uploadResult.SecureURL, nil
}
