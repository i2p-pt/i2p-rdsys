// Copyright (c) 2021-2022, The Tor Project, Inc.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gettor

import (
	"bytes"
	"github.com/stretchr/testify/assert"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/internal"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/usecases/resources"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestCreateUserToken(t *testing.T) {
	strFromEnv := func(key string) string {
		env, envOk := os.LookupEnv(key)
		if !envOk {
			t.Skipf("no %v, google drive create token test aborted", key)
		}
		return env
	}
	updater, err := newGoogleDriveUpdater(&internal.GoogleDriveUpdater{
		AppCredentialPath:  strFromEnv("GOOGLE_DRIVE_CREATE_TOKEN_APP_CREDENTIAL"),
		UserCredentialPath: strFromEnv("GOOGLE_DRIVE_CREATE_TOKEN_USER_CREDENTIAL"),
		ParentFolderID:     "",
	})
	assert.NoError(t, err)
	updaterInternal := updater.(*googleDriveUpdater)
	err = updaterInternal.createToken(strFromEnv("GOOGLE_DRIVE_CREATE_TOKEN_USER_AUTHCODE"))
	assert.NoError(t, err)
}

func TestUploadFile(t *testing.T) {
	strFromEnv := func(key string) string {
		env, envOk := os.LookupEnv(key)
		if !envOk {
			t.Skipf("no %v, google drive create token test aborted", key)
		}
		return env
	}
	updater, err := newGoogleDriveUpdater(&internal.GoogleDriveUpdater{
		AppCredentialPath:  strFromEnv("GOOGLE_DRIVE_UPLOAD_FILE_APP_CREDENTIAL"),
		UserCredentialPath: strFromEnv("GOOGLE_DRIVE_UPLOAD_FILE_USER_CREDENTIAL"),
		ParentFolderID:     strFromEnv("GOOGLE_DRIVE_UPLOAD_FILE_USER_FOLDER_ID"),
	})
	assert.NoError(t, err)
	updaterInternal := updater.(*googleDriveUpdater)

	buf := make([]byte, 1<<21)
	io.ReadFull(rand.New(rand.NewSource(time.Now().Unix())), buf)

	t.Run("upload", func(t *testing.T) {
		link, err := updaterInternal.uploadFileAndGetLink("testing", bytes.NewReader(buf))
		assert.NoError(t, err)
		t.Run("check file existence", func(t *testing.T) {
			exist, err := updaterInternal.checkFileExistence("testing")
			assert.NoError(t, err)
			assert.True(t, exist)
		})
		t.Run("download from link", func(t *testing.T) {
			resp, err := http.Get(link)
			assert.NoError(t, err)
			assert.Equal(t, 200, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			assert.NoError(t, err)
			assert.Equal(t, buf, body)
		})
	})
}

func TestGoogleDrive(t *testing.T) {
	strFromEnv := func(key string) string {
		env, envOk := os.LookupEnv(key)
		if !envOk {
			t.Skipf("no %v, google drive create token test aborted", key)
		}
		return env
	}
	updater, err := newGoogleDriveUpdater(&internal.GoogleDriveUpdater{
		AppCredentialPath:  strFromEnv("GOOGLE_DRIVE_TEST_APP_CREDENTIAL"),
		UserCredentialPath: strFromEnv("GOOGLE_DRIVE_TEST_USER_CREDENTIAL"),
		ParentFolderID:     strFromEnv("GOOGLE_DRIVE_TEST_USER_FOLDER_ID"),
	})
	assert.NoError(t, err)
	version := resources.Version{
		Mayor: int(time.Now().Unix()),
	}

	needUpdate := updater.needsUpdate("toros", version)
	assert.True(t, needUpdate)
	releaseFunc := updater.newRelease("toros", version)
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
			})
		}
	})

}
