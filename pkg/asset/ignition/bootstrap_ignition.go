package ignition

import (
	"encoding/json"

	igntypes "github.com/coreos/ignition/v2/config/v3_2/types"
	"github.com/openshift/appliance/pkg/asset/config"
	"github.com/openshift/appliance/pkg/asset/manifests"
	"github.com/openshift/appliance/pkg/templates"
	"github.com/openshift/installer/pkg/asset"
	"github.com/openshift/installer/pkg/asset/ignition/bootstrap"
	"github.com/openshift/installer/pkg/asset/password"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"
)

const (
	bootstrapRegistryDataPath = "/mnt/agentdata/oc-mirror/bootstrap"
	registriesConfFilePath    = "/etc/containers/registries.conf"
	manifestPath              = "/etc/assisted/manifests"
)

var (
	bootstrapServices = []string{
		"start-local-registry.service",
		"assisted-service.service",
		"create-cluster-and-infraenv.service",
		"pre-install.service",
		"update-hosts.service",
	}

	bootstrapScripts = []string{
		"start-local-registry.sh",
		"set-env-files.sh",
		"pre-install.sh",
		"release-image-download.sh",
		"release-image.sh",
		"update-hosts.sh",

		// TODO: remove (needed for using custom agent image)
		"get-container-images.sh",
	}
)

// BootstrapIgnition generates the bootstrap ignition file for the recovery ISO
type BootstrapIgnition struct {
	Config igntypes.Config
}

var _ asset.Asset = (*BootstrapIgnition)(nil)

// Name returns the human-friendly name of the asset.
func (i *BootstrapIgnition) Name() string {
	return "Bootstrap ignition"
}

// Dependencies returns dependencies used by the asset.
func (i *BootstrapIgnition) Dependencies() []asset.Asset {
	return []asset.Asset{
		&config.EnvConfig{},
		&config.ApplianceConfig{},
		&password.KubeadminPassword{},
		&manifests.ClusterImageSet{},
		&InstallIgnition{},
	}
}

// Generate the base ISO.
func (i *BootstrapIgnition) Generate(dependencies asset.Parents) error {
	envConfig := &config.EnvConfig{}
	applianceConfig := &config.ApplianceConfig{}
	pwd := &password.KubeadminPassword{}
	installIgnition := &InstallIgnition{}
	dependencies.Get(envConfig, applianceConfig, pwd, installIgnition)

	i.Config = igntypes.Config{
		Ignition: igntypes.Ignition{
			Version: igntypes.MaxVersion.String(),
		},
	}

	if envConfig.DebugBootstrap {
		// Avoid machine reboot after bootstrap to debug install ignition
		bootstrapServices = append(bootstrapServices, "ironic-agent.service")
	}

	// Add services common for bootstrap and install
	if err := bootstrap.AddSystemdUnits(&i.Config, "services/common", nil, bootstrapServices); err != nil {
		return err
	}

	// Add services exclusive for bootstrap
	if err := bootstrap.AddSystemdUnits(&i.Config, "services/bootstrap", nil, bootstrapServices); err != nil {
		return err
	}

	// Fetch install ignition config
	installIgnitionConfig, err := json.Marshal(installIgnition.Config)
	if err != nil {
		return err
	}

	// Add bootstrap scripts to ignition
	templateData := templates.GetBootstrapIgnitionTemplateData(
		applianceConfig.Config.OcpRelease, bootstrapRegistryDataPath, string(installIgnitionConfig))
	for _, script := range bootstrapScripts {
		if err := bootstrap.AddStorageFiles(&i.Config,
			"/usr/local/bin/"+script,
			"scripts/bin/"+script+".template",
			templateData); err != nil {
			return err
		}
	}

	passwdUser := igntypes.PasswdUser{
		Name: "core",
	}
	// Add user 'core' password
	if applianceConfig.Config.UserCorePass != nil {
		passBytes, err := bcrypt.GenerateFromPassword([]byte(*applianceConfig.Config.UserCorePass), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		pwdHash := string(passBytes)
		passwdUser.PasswordHash = &pwdHash
	}
	// Add public ssh key
	if applianceConfig.Config.SshKey != nil {
		passwdUser.SSHAuthorizedKeys = []igntypes.SSHAuthorizedKey{
			igntypes.SSHAuthorizedKey(*applianceConfig.Config.SshKey),
		}
	}
	i.Config.Passwd.Users = append(i.Config.Passwd.Users, passwdUser)

	logrus.Debug("Successfully generated bootstrap ignition")

	return nil
}
