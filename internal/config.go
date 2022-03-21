package internal

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
)

// Config represents our central configuration file.
type Config struct {
	Backend      BackendConfig `json:"backend"`
	Distributors Distributors  `json:"distributors"`
	Updaters     Updaters      `json:"updaters"`
}

type BackendConfig struct {
	ExtrainfoFile          string            `json:"extrainfo_file"`
	NetworkstatusFile      string            `json:"networkstatus_file"`
	DescriptorsFile        string            `json:"descriptors_file"`
	BlocklistFile          string            `json:"blocklist_file"`
	AllowlistFile          string            `json:"allowlist_file"`
	ApiTokens              map[string]string `json:"api_tokens"`
	ResourcesEndpoint      string            `json:"api_endpoint_resources"`
	ResourceStreamEndpoint string            `json:"api_endpoint_resource_stream"`
	TargetsEndpoint        string            `json:"api_endpoint_targets"`
	StatusEndpoint         string            `json:"web_endpoint_status"`
	MetricsEndpoint        string            `json:"web_endpoint_metrics"`
	BridgestrapEndpoint    string            `json:"bridgestrap_endpoint"`
	StorageDir             string            `json:"storage_dir"`
	AssignmentsFile        string            `json:"assignments_file"`
	// DistProportions contains the proportion of resources that each
	// distributor should get.  E.g. if the HTTPS distributor is set to x and
	// the Salmon distributor is set to y, then HTTPS gets x/(x+y) of all
	// resources and Salmon gets y/(x+y).
	DistProportions map[string]int            `json:"distribution_proportions"`
	Resources       map[string]ResourceConfig `json:"resources"`
	WebApi          WebApiConfig              `json:"web_api"`
}

type ResourceConfig struct {
	Unpartitioned bool     `json:"unpartitioned"`
	Stored        bool     `json:"stored"`
	Distributors  []string `json:"distributors"`
}

type Distributors struct {
	Https    HttpsDistConfig    `json:"https"`
	Salmon   SalmonDistConfig   `json:"salmon"`
	Stub     StubDistConfig     `json:"stub"`
	Gettor   GettorDistConfig   `json:"gettor"`
	Moat     MoatDistConfig     `json:"moat"`
	Telegram TelegramDistConfig `json:"telegram"`
}

type StubDistConfig struct {
	Resources []string     `json:"resources"`
	WebApi    WebApiConfig `json:"web_api"`
}

type HttpsDistConfig struct {
	Resources []string     `json:"resources"`
	WebApi    WebApiConfig `json:"web_api"`
}

type SalmonDistConfig struct {
	Resources  []string     `json:"resources"`
	WebApi     WebApiConfig `json:"web_api"`
	WorkingDir string       `json:"working_dir"` // This is where Salmon stores its state.
}

type GettorDistConfig struct {
	Resources []string    `json:"resources"`
	Email     EmailConfig `json:"email"`
}

type MoatDistConfig struct {
	Resources             []string     `json:"resources"`
	GeoipDB               string       `json:"geoipdb"`
	Geoip6DB              string       `json:"geoip6db"`
	CircumventionMap      string       `json:"circumvention_map"`
	CircumventionDefaults string       `json:"circumvention_defaults"`
	NumBridgesPerRequest  int          `json:"num_bridges_per_request"`
	RotationPeriodHours   int          `json:"rotation_period_hours"`
	NumPeriods            int          `json:"num_periods"`
	BuiltInBridgesURL     string       `json:"builtin_bridges_url"`
	BuiltInBridgesTypes   []string     `json:"builtin_bridges_types"`
	WebApi                WebApiConfig `json:"web_api"`
}

type TelegramDistConfig struct {
	Resource             string `json:"resource"`
	NumBridgesPerRequest int    `json:"num_bridges_per_request"`
	RotationPeriodHours  int    `json:"rotation_period_hours"`
	Token                string `json:"token"`
	MinUserID            int64  `json:"min_user_id"`
	NewBridgesFile       string `json:"new_bridges_file"`
}

type WebApiConfig struct {
	ApiAddress string `json:"api_address"`
	CertFile   string `json:"cert_file"`
	KeyFile    string `json:"key_file"`
}

type EmailConfig struct {
	Address      string `json:"address"`
	SmtpServer   string `json:"smtp_server"`
	SmtpUsername string `json:"smtp_username"`
	SmtpPassword string `json:"smtp_password"`
	ImapServer   string `json:"imap_server"`
	ImapUsername string `json:"imap_username"`
	ImapPassword string `json:"imap_password"`
}

type Updaters struct {
	Gettor GettorUpdater `json:"gettor"`
}

type GettorUpdater struct {
	Github             Github             `json:"github"`
	S3Updaters         []S3Updater        `json:"s3"`
	GoogleDriveUpdater GoogleDriveUpdater `json:"gdrive"`
}

type Github struct {
	AuthToken string `json:"auth_token"`
	Owner     string `json:"owner"`
	Repo      string `json:"repo"`
}

type S3Updater struct {
	AccessKey                    string `json:"access_key"`
	AccessSecret                 string `json:"access_secret"`
	SigningMethod                string `json:"signing_method"`
	EndpointUrl                  string `json:"endpoint_url"`
	EndpointRegion               string `json:"endpoint_region"`
	Name                         string `json:"name"`
	Bucket                       string `json:"bucket"`
	NameProceduralGenerationSeed string `json:"name_procedural_generation_seed"`
}

type GoogleDriveUpdater struct {
	AppCredentialPath  string `json:"app_credential_path"`
	UserCredentialPath string `json:"user_credential_path"`
	ParentFolderID     string `json:"parent_folder_id"`
}

// LoadConfig loads the given JSON configuration file and returns the resulting
// Config configuration object.
func LoadConfig(filename string) (*Config, error) {

	log.Printf("Attempting to load configuration file at %s.", filename)

	info, err := os.Stat(filename)
	if err != nil {
		return nil, err
	}
	if info.Mode() != 0600 {
		return nil, fmt.Errorf("file %s contains secrets and therefore must have 0600 permissions", filename)
	}

	content, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var config Config
	if err = json.Unmarshal(content, &config); err != nil {
		return nil, err
	}

	return &config, nil
}
