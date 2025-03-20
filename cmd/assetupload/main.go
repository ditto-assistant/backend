package main

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/ditto-assistant/backend/cfg/envs"
	"github.com/ditto-assistant/backend/cfg/secr"
)

var (
	sourcePath   = flag.String("source", "pkg/web/static/assets", "Source path for assets to upload")
	destPath     = flag.String("dest", "assets", "Destination path prefix in B2 bucket")
	dryRun       = flag.Bool("dry-run", false, "Dry run mode (don't actually upload)")
	concurrency  = flag.Int("concurrency", 5, "Number of concurrent uploads")
	forceUpload  = flag.Bool("force", false, "Force upload even if file exists and is identical")
	excludeGlobs = flag.String("exclude", ".DS_Store", "Comma-separated list of glob patterns to exclude")
	includeGz    = flag.Bool("include-gz", true, "Include gzipped versions of files")
)

// FileToUpload represents a file to be uploaded to B2
type FileToUpload struct {
	LocalPath    string
	DestKey      string
	ContentType  string
	CacheControl string
}

func main() {
	flag.Parse()

	if err := envs.Load(); err != nil {
		log.Fatalf("Failed to load environment variables: %v", err)
	}

	ctx := context.Background()
	if _, err := secr.Setup(ctx); err != nil {
		log.Fatalf("Failed to setup secrets: %v", err)
	}

	s3Config := &aws.Config{
		Credentials: credentials.NewStaticCredentials(envs.BACKBLAZE_KEY_ID, secr.BACKBLAZE_API_KEY.String(), ""),
		Region:      aws.String(envs.DITTO_CONTENT_REGION),
		Endpoint:    aws.String(envs.DITTO_CONTENT_ENDPOINT),
	}
	mySession, err := session.NewSession(s3Config)
	if err != nil {
		log.Fatalf("Failed to create AWS session: %v", err)
	}
	s3Client := s3.New(mySession)

	// Convert exclude globs to a slice
	excludePatterns := strings.Split(*excludeGlobs, ",")
	for i := range excludePatterns {
		excludePatterns[i] = strings.TrimSpace(excludePatterns[i])
	}

	// Scan for files to upload
	filesToUpload, err := scanFiles(*sourcePath, *destPath, excludePatterns, *includeGz)
	if err != nil {
		log.Fatalf("Failed to scan files: %v", err)
	}

	fmt.Printf("Found %d files to upload to %s\n", len(filesToUpload), envs.DITTO_CONTENT_BUCKET)
	if *dryRun {
		fmt.Println("Running in dry-run mode, no files will be uploaded")
		for _, file := range filesToUpload {
			fmt.Printf("Would upload %s to %s (%s)\n", file.LocalPath, file.DestKey, file.ContentType)
		}
		return
	}

	// Channel for files to upload and semaphore for concurrency control
	filesChan := make(chan FileToUpload, len(filesToUpload))
	var wg sync.WaitGroup

	// Start worker goroutines
	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for file := range filesChan {
				if err := uploadFile(ctx, s3Client, file); err != nil {
					log.Printf("Error uploading %s: %v", file.LocalPath, err)
				}
			}
		}()
	}

	// Queue files for upload
	for _, file := range filesToUpload {
		filesChan <- file
	}
	close(filesChan)

	// Wait for all uploads to finish
	wg.Wait()
	fmt.Println("Upload complete!")
}

// scanFiles walks the source directory and prepares files for upload
func scanFiles(sourcePath, destPath string, excludePatterns []string, includeGz bool) ([]FileToUpload, error) {
	var files []FileToUpload

	err := filepath.Walk(sourcePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		// Get the base filename
		baseFile := filepath.Base(path)
		
		// Skip .gz files unless explicitly included
		if !includeGz && strings.HasSuffix(baseFile, ".gz") {
			return nil
		}
		
		// Check if file matches any exclude pattern
		for _, pattern := range excludePatterns {
			if matched, _ := filepath.Match(pattern, baseFile); matched {
				return nil
			}
		}

		// Build destination key
		relPath, err := filepath.Rel(sourcePath, path)
		if err != nil {
			return err
		}

		destKey := filepath.Join(destPath, relPath)
		// Convert path separators to forward slashes for S3
		destKey = strings.ReplaceAll(destKey, "\\", "/")

		// Determine content type
		contentType := mime.TypeByExtension(filepath.Ext(path))
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		
		// Handle .gz files - set the content type based on the original file
		if strings.HasSuffix(path, ".gz") {
			// Extract the original extension
			origExt := filepath.Ext(strings.TrimSuffix(path, ".gz"))
			if origExt != "" {
				// Get content type for the original file
				origContentType := mime.TypeByExtension(origExt)
				if origContentType != "" {
					contentType = origContentType
				}
			}
		}

		// Set cache control based on file type
		cacheControl := "public, max-age=86400" // Default 1 day
		ext := strings.ToLower(filepath.Ext(strings.TrimSuffix(path, ".gz")))
		if ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".webp" || ext == ".svg" || ext == ".ico" {
			cacheControl = "public, max-age=604800" // 7 days for images
		}

		files = append(files, FileToUpload{
			LocalPath:    path,
			DestKey:      destKey,
			ContentType:  contentType,
			CacheControl: cacheControl,
		})

		return nil
	})

	return files, err
}

// uploadFile uploads a single file to B2
func uploadFile(ctx context.Context, s3Client *s3.S3, file FileToUpload) error {
	// Check if file already exists with the same ETag to avoid unnecessary uploads
	if !*forceUpload {
		localHash, err := calculateMD5(file.LocalPath)
		if err != nil {
			return fmt.Errorf("failed to calculate MD5 for %s: %w", file.LocalPath, err)
		}

		headOutput, err := s3Client.HeadObjectWithContext(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(envs.DITTO_CONTENT_BUCKET),
			Key:    aws.String(file.DestKey),
		})

		// If file exists and has the same ETag, skip upload
		if err == nil && headOutput.ETag != nil {
			remoteETag := strings.Trim(*headOutput.ETag, "\"")
			if remoteETag == localHash {
				fmt.Printf("Skipping %s (already exists with same hash)\n", file.DestKey)
				return nil
			}
		}
	}

	// Read file content
	content, err := os.ReadFile(file.LocalPath)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", file.LocalPath, err)
	}

	// Prepare upload input
	uploadInput := &s3.PutObjectInput{
		Bucket:       aws.String(envs.DITTO_CONTENT_BUCKET),
		Key:          aws.String(file.DestKey),
		Body:         bytes.NewReader(content),
		ContentType:  aws.String(file.ContentType),
		CacheControl: aws.String(file.CacheControl),
		// Ensure public read access
		ACL: aws.String("public-read"),
	}
	
	// If file is gzipped, add the Content-Encoding header
	if strings.HasSuffix(file.LocalPath, ".gz") {
		uploadInput.ContentEncoding = aws.String("gzip")
	}
	
	// Upload file
	_, err = s3Client.PutObjectWithContext(ctx, uploadInput)

	if err != nil {
		return fmt.Errorf("failed to upload %s: %w", file.DestKey, err)
	}

	fmt.Printf("Uploaded %s to %s (%s, %s)\n", 
		file.LocalPath, file.DestKey, file.ContentType, file.CacheControl)
	return nil
}

// calculateMD5 calculates the MD5 hash of a file
func calculateMD5(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Read file content
	fileContent, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}

	// Calculate MD5 hash
	hash := md5.New()
	hash.Write(fileContent)
	hashBytes := hash.Sum(nil)
	
	// Convert to hex string (S3 ETags are hex MD5 sums without quotes)
	return hex.EncodeToString(hashBytes), nil
}