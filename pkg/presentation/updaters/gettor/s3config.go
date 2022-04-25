// Copyright (c) 2021-2022, The Tor Project, Inc.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gettor

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/internal"
)

type s3Object struct {
	bucket string
	name   string
}

type s3ConfigAdaptor struct {
	internal.S3Updater
	signer *v4.Signer
}

var errUnknownSigningMethod = errors.New("signing method is not recognized")

func newS3ConfigAdaptor(cfg internal.S3Updater) s3ConfigAdaptor {
	return s3ConfigAdaptor{S3Updater: cfg, signer: v4.NewSigner()}
}

func (s s3ConfigAdaptor) SignHTTP(ctx context.Context, credentials aws.Credentials, r *http.Request, payloadHash string, service string, region string, signingTime time.Time, optFns ...func(*v4.SignerOptions)) error {
	switch s.SigningMethod {
	case "v4":
		return s.signer.SignHTTP(ctx, credentials, r, payloadHash, service, region, signingTime, optFns...)
	case "archive_org_dangerous_workaround":
		// This is a workaround to compensate for archive.org's API's inability to handle
		// DO NOT use this on other providers
		// https://github.com/rclone/rclone/issues/1608
		r.Header.Set("Authorization", fmt.Sprintf("LOW %v:%v", credentials.AccessKeyID, credentials.SecretAccessKey))

		// These are used to workaround archive.org's API imperfections.
		r.Header.Del("X-Amz-Content-Sha256")
		r.Header.Del("Amz-Sdk-Invocation-Id")
		r.Header.Del("Amz-Sdk-Request")
		r.Header.Del("Content-Type")
		r.Header.Del("Accept-Encoding")

		r.URL.RawQuery = ""

		r.Header.Set("Accept", "*/*")
		r.Header.Set("Content-Length", strconv.FormatInt(r.ContentLength, 10))

		return nil
	default:
		return errUnknownSigningMethod
	}
}

func (s s3ConfigAdaptor) PresignHTTP(ctx context.Context, credentials aws.Credentials, r *http.Request, payloadHash string, service string, region string, signingTime time.Time, optFns ...func(*v4.SignerOptions)) (url string, signedHeader http.Header, err error) {
	switch s.SigningMethod {
	case "v4":
		return s.signer.PresignHTTP(ctx, credentials, r, payloadHash, service, region, signingTime, optFns...)
	default:
		return "", nil, errUnknownSigningMethod
	}
}

func (s s3ConfigAdaptor) Retrieve(ctx context.Context) (aws.Credentials, error) {
	return aws.Credentials{AccessKeyID: s.AccessKey, SecretAccessKey: s.AccessSecret}, nil
}

func (s s3ConfigAdaptor) ResolveEndpoint(service, region string) (aws.Endpoint, error) {
	return aws.Endpoint{
		URL:           s.EndpointUrl,
		Source:        aws.EndpointSourceCustom,
		SigningRegion: s.EndpointRegion,
		SigningMethod: s.SigningMethod,
		// HostnameImmutable = true ensures the bucket name does not appear in client requests' SNI
		HostnameImmutable: true,
	}, nil
}

func constructS3ClientFromConfig(cfg internal.S3Updater) *s3.Client {
	awsConfig := aws.NewConfig()
	awsConfig.EndpointResolver = newS3ConfigAdaptor(cfg)
	awsConfig.Credentials = newS3ConfigAdaptor(cfg)
	s3Client := newS3FromConfig(cfg, *awsConfig)
	return s3Client
}

type wrappedEndpointResolver struct {
	s3ConfigAdaptor
}

func (w wrappedEndpointResolver) ResolveEndpoint(region string, options s3.EndpointResolverOptions) (aws.Endpoint, error) {
	return w.s3ConfigAdaptor.ResolveEndpoint("s3", region)
}

// NewS3FromConfig returns a new client from the provided config.
// Modified based on github.com/aws/aws-sdk-go-v2/service/s3@v1.17.0/api_client.go
func newS3FromConfig(icfg internal.S3Updater, cfg aws.Config, optFns ...func(*s3.Options)) *s3.Client {
	opts := s3.Options{
		Region:           cfg.Region,
		EndpointResolver: wrappedEndpointResolver{newS3ConfigAdaptor(icfg)},
		HTTPClient:       cfg.HTTPClient,
		Credentials:      cfg.Credentials,
		APIOptions:       cfg.APIOptions,
		Logger:           cfg.Logger,
		ClientLogMode:    cfg.ClientLogMode,
		HTTPSignerV4:     newS3ConfigAdaptor(icfg),
	}
	return s3.New(opts, optFns...)
}
