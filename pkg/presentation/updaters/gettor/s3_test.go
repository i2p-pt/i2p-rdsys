// Copyright (c) 2021-2022, The Tor Project, Inc.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gettor

import (
	"bytes"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/internal"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/usecases/resources"
)

func TestS3(t *testing.T) {
	s3Key, s3KeyOk := os.LookupEnv("S3_TEST_ACCESS_KEY")
	if !s3KeyOk {
		t.Skip("no S3_TEST_ACCESS_KEY, s3 test aborted")
	}
	s3Secret, s3SecretOk := os.LookupEnv("S3_TEST_ACCESS_SECRET")
	if !s3SecretOk {
		t.Skip("no S3_TEST_ACCESS_SECRET, s3 test aborted")
	}
	s3Endpoint, s3EndpointOk := os.LookupEnv("S3_TEST_ENDPOINT")
	if !s3EndpointOk {
		t.Skip("no S3_TEST_ENDPOINT, s3 test aborted")
	}
	s3EndpointRegion, _ := os.LookupEnv("S3_TEST_ENDPOINT_REGION")

	s3Bucket, s3BucketOk := os.LookupEnv("S3_TEST_BUCKET")
	if !s3BucketOk {
		t.Skip("no S3_TEST_BUCKET, s3 test aborted")
	}

	s3SigningMethod, s3SigningMethodOk := os.LookupEnv("S3_TEST_SIGNINGMETHOD")
	if !s3SigningMethodOk {
		t.Skip("no S3_TEST_SIGNINGMETHOD, s3 test aborted")
	}

	updater := internal.S3Updater{
		AccessKey:      s3Key,
		AccessSecret:   s3Secret,
		SigningMethod:  s3SigningMethod,
		EndpointUrl:    s3Endpoint,
		EndpointRegion: s3EndpointRegion,
		Name:           "testing",
	}
	buf := make([]byte, 1<<21)

	io.ReadFull(rand.New(rand.NewSource(time.Now().Unix())), buf)

	s3Updater, _ := newS3Updater(&updater)

	updaterInternal := s3Updater.(s3updater)

	t.Run("upload", func(t *testing.T) {
		dataObject := s3Object{
			bucket: s3Bucket,
			name:   "test",
		}

		t.Run("create object", func(t *testing.T) {
			err := updaterInternal.createObject(dataObject, bytes.NewReader(buf))
			assert.NoError(t, err)
		})

		t.Run("object exists", func(t *testing.T) {
			err := updaterInternal.checkObjectExistence(dataObject)
			assert.NoError(t, err)
		})

		t.Run("create link", func(t *testing.T) {
			link, err := updaterInternal.createLink(dataObject)
			assert.NoError(t, err)
			t.Log(link)

			t.Run("download from link", func(t *testing.T) {
				resp, err := http.Get(link)
				assert.NoError(t, err)
				assert.Equal(t, 200, resp.StatusCode)

				body, err := io.ReadAll(resp.Body)
				assert.NoError(t, err)
				assert.Equal(t, buf, body)
			})
		})
	})

	t.Run("missing object does not exist", func(t *testing.T) {
		missingObject := s3Object{
			bucket: s3Bucket,
			name:   "test" + time.Now().String(),
		}

		err := updaterInternal.checkObjectExistence(missingObject)
		assert.Error(t, err)
	})

}

func TestNameGeneration(t *testing.T) {
	updater := internal.S3Updater{
		Name:                         "testing",
		NameProceduralGenerationSeed: "1234",
	}
	s3Updater, _ := newS3Updater(&updater)
	updaterInternal := s3Updater.(s3updater)
	existenceObj := updaterInternal.formatNameForExistenceObject("toros", resources.Version{
		Mayor: 1,
		Minor: 2,
		Patch: 3,
	})
	assert.True(t, strings.HasSuffix(existenceObj.name, ".exist-gettor"))
	t.Log(existenceObj.name)
	assert.GreaterOrEqual(t, len(existenceObj.bucket), 15)
	t.Log(existenceObj.bucket)

	t.Run("reproducible", func(t *testing.T) {
		updater := internal.S3Updater{
			Name:                         "testing",
			NameProceduralGenerationSeed: "1234",
		}
		s3Updater, _ := newS3Updater(&updater)
		updaterInternal := s3Updater.(s3updater)
		existenceObj2 := updaterInternal.formatNameForExistenceObject("toros", resources.Version{
			Mayor: 1,
			Minor: 2,
			Patch: 3,
		})
		assert.Equal(t, existenceObj2.bucket, existenceObj.bucket)
	})
}

func TestArchiveOrg(t *testing.T) {
	// WARNING: This test takes significant times. ~3 mins
	if testing.Short() {
		t.SkipNow()
	}
	s3Key, s3KeyOk := os.LookupEnv("ARCHIVE_ORG_TEST_ACCESS_KEY")
	if !s3KeyOk {
		t.Skip("no ARCHIVE_ORG_TEST_ACCESS_KEY, s3 test aborted")
	}
	s3Secret, s3SecretOk := os.LookupEnv("ARCHIVE_ORG_TEST_ACCESS_SECRET")
	if !s3SecretOk {
		t.Skip("no ARCHIVE_ORG_TEST_ACCESS_SECRET, s3 test aborted")
	}

	updater := internal.S3Updater{
		AccessKey:      s3Key,
		AccessSecret:   s3Secret,
		SigningMethod:  "archive_org_dangerous_workaround",
		EndpointUrl:    "https://s3.us.archive.org/",
		EndpointRegion: "",
		Name:           "archive_org",
	}
	version := resources.Version{
		Mayor: int(time.Now().Unix()),
	}
	s3Updater, _ := newS3Updater(&updater)
	needUpdate := s3Updater.needsUpdate("toros", version)
	assert.True(t, needUpdate)
	releaseFunc := s3Updater.newRelease("toros", version)

	t.Run("upload files", func(t *testing.T) {
		{
			tmpDir, err := os.MkdirTemp("", "gettor-test-")
			assert.NoError(t, err)
			defer os.RemoveAll(tmpDir)

			tmpDataFile, err := os.Create(tmpDir + "/file")
			assert.NoError(t, err)
			dataFileContent := make([]byte, 1<<21)
			io.Copy(tmpDataFile, bytes.NewReader(dataFileContent))
			dataFilePath := tmpDataFile.Name()
			tmpDataFile.Close()

			tmpSignatureFile, err := os.Create(tmpDir + "/sigfile")
			assert.NoError(t, err)
			signatureFileContent := make([]byte, 1<<11)
			io.Copy(tmpSignatureFile, bytes.NewReader(signatureFileContent))
			signatureFilePath := tmpSignatureFile.Name()
			tmpSignatureFile.Close()

			releaseLinkDataResource := releaseFunc(dataFilePath, signatureFilePath, "testingLocation")

			// Now wait until the data is installed
			func() {
				for i := 0; i <= 10; i++ {
					time.Sleep(time.Second * 20)
					resp, err := http.Get(releaseLinkDataResource.Link)
					if err != nil {
						continue
					}
					body, _ := io.ReadAll(resp.Body)
					if bytes.Equal(body, dataFileContent) {
						return
					}
				}
				t.Fatal("upload to archive org is unsuccessful: data")
			}()

			// Now wait until the data is installed
			func() {
				for i := 0; i <= 10; i++ {
					time.Sleep(time.Second * 20)
					resp, err := http.Get(releaseLinkDataResource.SigLink)
					if err != nil {
						continue
					}
					body, _ := io.ReadAll(resp.Body)
					if bytes.Equal(body, signatureFileContent) {
						return
					}
				}
				t.Fatal("upload to archive org is unsuccessful: sign")
			}()
		}
	})

}

func TestStandardS3(t *testing.T) {
	strFromEnv := func(key string) string {
		env, envOk := os.LookupEnv(key)
		if !envOk {
			t.Skipf("no %v, s3 test aborted", key)
		}
		return env
	}
	testSuites := []struct {
		name           string
		accessKey      string
		accessSecret   string
		bucket         string
		endpointUrl    string
		endpointRegion string
	}{
		{
			name:           "scaleway",
			accessKey:      strFromEnv("SCALEWAY_S3_TEST_ACCESS_KEY"),
			accessSecret:   strFromEnv("SCALEWAY_S3_TEST_ACCESS_SECRET"),
			bucket:         strFromEnv("SCALEWAY_S3_TEST_BUCKET"),
			endpointUrl:    "https://s3.fr-par.scw.cloud",
			endpointRegion: "fr-par",
		},
	}
	for _, s3ToBeTested := range testSuites {
		t.Run(s3ToBeTested.name, func(t *testing.T) {
			updater := internal.S3Updater{
				AccessKey:      s3ToBeTested.accessKey,
				AccessSecret:   s3ToBeTested.accessSecret,
				SigningMethod:  "v4",
				EndpointUrl:    s3ToBeTested.endpointUrl,
				EndpointRegion: s3ToBeTested.endpointRegion,
				Name:           s3ToBeTested.name,
				Bucket:         s3ToBeTested.bucket,
			}
			version := resources.Version{
				Mayor: int(time.Now().Unix()),
			}
			s3Updater, _ := newS3Updater(&updater)
			needUpdate := s3Updater.needsUpdate("toros", version)
			assert.True(t, needUpdate)
			releaseFunc := s3Updater.newRelease("toros", version)
			t.Run("upload files", func(t *testing.T) {
				for fileIndex := 0; fileIndex <= 5; fileIndex++ {
					strFileIndex := strconv.FormatInt(int64(fileIndex), 10)
					t.Run(strFileIndex, func(t *testing.T) {

						tmpdir, err := os.MkdirTemp("", "gettor-test-")
						assert.NoError(t, err)
						defer os.RemoveAll(tmpdir)

						tmpfile, err := os.Create(tmpdir + "/file" + strFileIndex)
						assert.NoError(t, err)
						buf := make([]byte, 1<<21)
						io.Copy(tmpfile, bytes.NewReader(buf))
						filename := tmpfile.Name()
						tmpfile.Close()

						tmpsigfile, err := os.Create(tmpdir + "/sigfile" + strFileIndex)
						assert.NoError(t, err)
						bufsig := make([]byte, 1<<11)
						io.Copy(tmpsigfile, bytes.NewReader(bufsig))
						filesigname := tmpsigfile.Name()
						tmpsigfile.Close()

						release_data := releaseFunc(filename, filesigname, "testingLocation"+strFileIndex)

						t.Run("data file links works", func(t *testing.T) {
							resp, err := http.Get(release_data.Link)
							assert.NoError(t, err)
							assert.Equal(t, 200, resp.StatusCode)

							body, err := io.ReadAll(resp.Body)
							assert.NoError(t, err)
							assert.Equal(t, buf, body)
						})

						t.Run("sign file links works", func(t *testing.T) {
							resp, err := http.Get(release_data.SigLink)
							assert.NoError(t, err)
							assert.Equal(t, 200, resp.StatusCode)

							body, err := io.ReadAll(resp.Body)
							assert.NoError(t, err)
							assert.Equal(t, bufsig, body)
						})

						t.Run("update link only release test", func(t *testing.T) {
							releaseFuncUpdateOnly := s3Updater.newRelease("toros", version)
							release_data_updateOnly := releaseFuncUpdateOnly(filename, filesigname, "testingLocation"+strFileIndex)

							t.Run("data file links works", func(t *testing.T) {
								resp, err := http.Get(release_data.Link)
								assert.NoError(t, err)
								assert.Equal(t, 200, resp.StatusCode)

								body, err := io.ReadAll(resp.Body)
								assert.NoError(t, err)
								assert.Equal(t, buf, body)
							})

							t.Run("sign file links works", func(t *testing.T) {
								resp, err := http.Get(release_data.SigLink)
								assert.NoError(t, err)
								assert.Equal(t, 200, resp.StatusCode)

								body, err := io.ReadAll(resp.Body)
								assert.NoError(t, err)
								assert.Equal(t, bufsig, body)
							})

							assert.NotEqual(t, release_data.Link, release_data_updateOnly.Link)
							assert.NotEqual(t, release_data.SigLink, release_data_updateOnly.SigLink)
							assert.Equal(t, release_data.Oid(), release_data_updateOnly.Oid())
						})

					})
				}
			})
		})
	}
}
