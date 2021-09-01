package gettor

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"

	"github.com/xanzy/go-gitlab"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/internal"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/usecases/resources"
)

const (
	gitlabPlatform = "gitlab"
)

type gitlabProvider struct {
	client *gitlab.Client
	cfg    *internal.Gitlab
}

func newGitlabProvider(cfg *internal.Gitlab) (*gitlabProvider, error) {
	client, err := gitlab.NewClient(cfg.AuthToken)
	return &gitlabProvider{client, cfg}, err
}

func (gl *gitlabProvider) needsUpdate(platform string, version resources.Version) bool {
	project, _, err := gl.client.Projects.GetProject(gl.getProjectId(platform), nil)
	if err != nil {
		log.Println("[Gitlab] Error fetching project:", err)
		return true
	}

	releaseVersion, err := resources.Str2Version(project.Description)
	if err != nil {
		log.Println("[Gitlab] The previous version is not valid:", project.Description)
		return true
	}

	if version.Compare(releaseVersion) == 1 {
		log.Println("[Gitlab] needs update for", platform)
		return true
	}

	return false
}

func (gl *gitlabProvider) newRelease(platform string, version resources.Version) uploadFileFunc {
	branch := "main"

	_, err := gl.client.Projects.DeleteProject(gl.getProjectId(platform))
	if err != nil {
		log.Println("[Gitlab] Can't delete project", platform, ":", err)
	}

	description := version.String()
	visibility := gitlab.PublicVisibility
	projectOptions := gitlab.CreateProjectOptions{
		Name:          &platform,
		Description:   &description,
		DefaultBranch: &branch,
		Visibility:    &visibility,
	}
	_, _, err = gl.client.Projects.CreateProject(&projectOptions)
	if err != nil {
		log.Println("[Gitlab] Can't create project", platform, ":", err)
		return nil
	}

	return func(binaryPath string, sigPath string, locale string) *resources.TBLink {
		link := resources.NewTBLink()

		for i, filePath := range []string{binaryPath, sigPath} {
			f, err := os.Open(filePath)
			if err != nil {
				log.Println("[Gitlab] Couldn't open file", filePath, ":", err)
				return nil
			}
			defer f.Close()
			b, err := ioutil.ReadAll(f)
			if err != nil {
				log.Println("[Gitlab] Couldn't read file", filePath, ":", err)
				return nil
			}
			content := base64.StdEncoding.EncodeToString(b)

			filename := path.Base(filePath)
			encoding := "base64"
			fileOptions := gitlab.CreateFileOptions{
				Branch:        &branch,
				CommitMessage: &filename,
				Encoding:      &encoding,
				Content:       &content,
			}
			_, _, err = gl.client.RepositoryFiles.CreateFile(gl.getProjectId(platform), filename, &fileOptions)
			if err != nil {
				log.Println("[Gitlab] Couldn't upload file", filePath, ":", err)
				return nil
			}

			url := fmt.Sprintf("https://gitlab.com/%s/%s/-/raw/%s/%s?inline=false", gl.cfg.Owner, platform, branch, filename)
			if i == 0 {
				link.Link = url
			} else {
				link.SigLink = url
			}
		}

		link.Version = version
		link.Provider = gitlabPlatform
		link.Platform = platform
		link.Locale = locale
		link.FileName = path.Base(binaryPath)
		return link
	}
}

func (gl *gitlabProvider) getProjectId(platform string) string {
	return gl.cfg.Owner + "/" + platform
}
