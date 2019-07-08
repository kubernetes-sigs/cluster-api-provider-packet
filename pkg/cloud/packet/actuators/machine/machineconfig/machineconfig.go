package machineconfig

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"

	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/yaml"
)

type Getter interface {
	GetUserdata(string, clusterv1.MachineVersionInfo) (string, error)
}

// config is a single machine setup config that has userdata and parameters such as image name.
type config struct {
	// Params is a list of valid combinations of ConfigParams that will map to the appropriate Userdata.
	Params []*ConfigParams `json:"machineParams"`

	// Userdata is a script used to provision instance.
	Userdata string `json:"userdata"`
}

// configList is list of configs.
type configList struct {
	Items []config `json:"items"`
}
type ConfigParams struct {
	Image    string                       `json:"image"`
	Versions clusterv1.MachineVersionInfo `json:"versions"`
}

type GetterFile struct {
	path string
}

// GetUserdata gets the userdata for the given machine spec, or an error if none is found
func (g *GetterFile) GetUserdata(image string, versions clusterv1.MachineVersionInfo) (string, error) {
	if image == "" {
		return "", fmt.Errorf("invalid empty image requested")
	}
	f, err := os.Open(g.path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	c, err := loadConfigs(f)
	if err != nil {
		return "", fmt.Errorf("unable to load configs from file %s: %v", g.path, err)
	}
	// now find the config we want
	params := &ConfigParams{
		Image:    image,
		Versions: versions,
	}
	config, err := findMatchingConfig(c, params)
	if err != nil {
		return "", fmt.Errorf("unable to find matching config: %v", err)
	}

	return config.Userdata, nil

}

func loadConfigs(reader io.Reader) (*configList, error) {
	// read the file and get the data
	b, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	// read the yaml
	c := &configList{}
	err = yaml.Unmarshal(b, c)
	if err != nil {
		return nil, fmt.Errorf("unable to parse yaml content: %v", err)
	}
	return c, nil
}

func findMatchingConfig(configs *configList, params *ConfigParams) (*config, error) {
	matchingConfigs := make([]config, 0)
	for _, conf := range configs.Items {
		for _, validParams := range conf.Params {
			if params.Image != validParams.Image {
				continue
			}
			if params.Versions != validParams.Versions {
				continue
			}
			matchingConfigs = append(matchingConfigs, conf)
		}
	}

	if len(matchingConfigs) == 1 {
		return &matchingConfigs[0], nil
	}

	return nil, fmt.Errorf("could not find setup configs for params %#v", params)
}

func NewFileGetter(p string) (*GetterFile, error) {
	if p == "" {
		return nil, fmt.Errorf("cannot use empty file path")
	}
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file does not exist: %s", p)
	}
	return &GetterFile{
		path: p,
	}, nil
}
