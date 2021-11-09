package gettor

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"hash/crc64"
	"io"
	"log"
	"os"
	"path"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/internal"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/core"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/usecases/resources"
)

func newS3Updater(cfg *internal.S3Updater) (provider, error) {
	s3Client := constructS3ClientFromConfig(*cfg)
	return s3updater{config: cfg, s3: s3Client, ctx: context.Background()}, nil
}

type s3updater struct {
	config *internal.S3Updater
	s3     *s3.Client
	ctx    context.Context
}

func (s s3updater) needsUpdate(platform string, version resources.Version) bool {
	// Links will expire in 1 day, timely update to the link is required
	return true
}

func (s s3updater) newRelease(platform string, version resources.Version) uploadFileFunc {
	existenceObject := s.formatNameForExistenceObject(platform, version)
	var updateLinkOnly = false
	if s.checkObjectExistence(existenceObject) == nil {
		updateLinkOnly = true
	}
	if err := s.createObject(existenceObject, bytes.NewReader([]byte{0x00})); err != nil {
		log.Println("[S3] Unable to create existence object", err)
		return nil
	}

	return func(binaryPath string, sigPath string, locale string) *resources.TBLink {
		link := resources.NewTBLink()

		const binaryFile = 0
		const signatureFile = 1
		for i, filePath := range []string{binaryPath, sigPath} {
			filename := path.Base(filePath)
			objectName := s.formatNameForFile(platform, version, filename)
			fd, err := os.Open(filePath)
			if err != nil {
				log.Println("[S3] Unable to create file to be uploaded", err)
				return nil
			}
			defer fd.Close()
			if !updateLinkOnly {
				err = s.createObject(objectName, fd)
				if err != nil {
					log.Println("[S3] Unable to upload file ", err)
					return nil
				}
			}
			downloadLink, err := s.createLink(objectName)
			if err != nil {
				log.Println("[S3] Unable to get file link ", err)
				return nil
			}
			switch i {
			case binaryFile:
				link.Link = downloadLink
			case signatureFile:
				link.SigLink = downloadLink
			default:
				log.Println("[S3] unexpected file index")
				return nil
			}
		}

		link.Version = version
		link.Provider = s.config.Name
		link.Platform = platform
		link.Locale = locale
		link.FileName = path.Base(binaryPath)

		var duration = time.Hour * 24
		link.CustomExpiry = &duration

		fileid := fmt.Sprintf("version:%v, provider: %v, plafrorm: %v, locale: %v, filename: %v",
			link.Version, link.Provider, link.Platform, link.Locale, link.FileName)
		table := crc64.MakeTable(resources.Crc64Polynomial)
		var oid = core.Hashkey(crc64.Checksum([]byte(fileid), table))
		link.CustomOid = &oid
		return link
	}
}

func (s s3updater) checkObjectExistence(obj s3Object) error {
	{
		_, err := s.s3.HeadBucket(s.ctx, &s3.HeadBucketInput{
			Bucket: &obj.bucket,
		})
		if err != nil {
			return err
		}
	}
	{
		_, err := s.s3.HeadObject(s.ctx, &s3.HeadObjectInput{
			Bucket: &obj.bucket,
			Key:    &obj.name,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s s3updater) ensureBucketExist(bucket string) error {
	{
		_, err := s.s3.HeadBucket(s.ctx, &s3.HeadBucketInput{
			Bucket: &bucket,
		})
		if err != nil {
			_, errCreateBucket := s.s3.CreateBucket(s.ctx, &s3.CreateBucketInput{Bucket: &bucket})
			return errCreateBucket
		}
	}
	return nil
}

func (s s3updater) createObject(obj s3Object, content io.Reader) error {
	if err := s.ensureBucketExist(obj.bucket); err != nil &&
		// This is a workaround to compensate for archive.org's API's lack of read-after-write consistency
		s.config.SigningMethod != "archive_org_dangerous_workaround" {
		return err
	}

	_, err := s.s3.PutObject(s.ctx,
		&s3.PutObjectInput{Key: &obj.name, Bucket: &obj.bucket, Body: content})
	return err
}

func (s s3updater) withPersigner(options *s3.PresignOptions) {
	options.Presigner = newS3ConfigAdaptor(*s.config)
}

func (s s3updater) createLink(obj s3Object) (string, error) {
	if s.config.SigningMethod == "archive_org_dangerous_workaround" {
		// This is a workaround to compensate for archive.org's API's low performance on s3 endpoint
		// https://archive.org/services/docs/api/ias3.html#fast-get-downloads
		return fmt.Sprintf("https://archive.org/download/%v/%v", obj.bucket, obj.name), nil
	}
	persignClient := s3.NewPresignClient(s.s3, s.withPersigner)
	presignedResult, err := persignClient.PresignGetObject(s.ctx,
		&s3.GetObjectInput{Key: &obj.name, Bucket: &obj.bucket}, s3.WithPresignExpires(time.Hour*24))
	if err != nil {
		return "", err
	}
	return presignedResult.URL, nil
}

func (s s3updater) formatNameForExistenceObject(platform string, version resources.Version) s3Object {
	bucketName := s.createProcedurallyGeneratedName(
		fmt.Sprintf("%v,%v,%v,%v", platform, version, "tor-s3", s.config.Name))
	if s.config.Bucket != "" {
		bucketName = s.config.Bucket
	}
	return s3Object{name: fmt.Sprintf("%v-%v.exist-gettor", platform, version.String()),
		bucket: bucketName}
}

func (s s3updater) formatNameForFile(platform string, version resources.Version, filename string) s3Object {
	bucketName := s.createProcedurallyGeneratedName(
		fmt.Sprintf("%v,%v,%v,%v", platform, version, "tor-s3", s.config.Name))
	if s.config.Bucket != "" {
		bucketName = s.config.Bucket
	}
	return s3Object{name: fmt.Sprintf("%v", filename),
		bucket: bucketName}
}

func (s s3updater) createProcedurallyGeneratedName(input string) string {
	nameHmac := hmac.New(func() hash.Hash {
		return sha256.New()
	}, []byte(s.config.NameProceduralGenerationSeed))
	nameHmac.Write([]byte(input))
	result := nameHmac.Sum(nil)
	return hex.EncodeToString(result)[:16]
}
