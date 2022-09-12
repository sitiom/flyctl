package scanner

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
)

func configureBridgetown(sourceDir string) (*SourceInfo, error) {
	if !checksPass(sourceDir, dirContains("Gemfile", "bridgetown")) {
		return nil, nil
	}

	vars := make(map[string]interface{})

	s := &SourceInfo{
		Family: "Bridgetown",
		Port:   4000,
		Statics: []Static{
			{
				GuestPath: "/app/output",
				UrlPrefix: "/",
			},
		},
		Env: map[string]string{
			"PORT": "4000",
		},
		SkipDatabase: true,
	}

	var rubyVersion string
	var bundlerVersion string
	var nodeVersion string = "16.17.0"

	out, err := exec.Command("node", "-v").Output()

	if err == nil {
		nodeVersion = strings.TrimSpace(string(out))
		if nodeVersion[:1] == "v" {
			nodeVersion = nodeVersion[1:]
		}
	}

	rubyVersion, err = extractRubyVersion("Gemfile.lock", "Gemfile", ".ruby_version")

	if err != nil || rubyVersion == "" {
		rubyVersion = "3.1.2"

		out, err := exec.Command("ruby", "-v").Output()
		if err == nil {

			version := strings.TrimSpace(string(out))
			re := regexp.MustCompile(`ruby (?P<version>[\d.]+)`)
			m := re.FindStringSubmatch(version)

			for i, name := range re.SubexpNames() {
				if len(m) > 0 && name == "version" {
					rubyVersion = m[i]
				}
			}
		}
	}

	bundlerVersion, err = extractBundlerVersion("Gemfile.lock")

	if err != nil || bundlerVersion == "" {
		bundlerVersion = "2.3.21"

		out, err := exec.Command("bundle", "-v").Output()
		if err == nil {

			version := strings.TrimSpace(string(out))
			re := regexp.MustCompile(`Bundler version (?P<version>[\d.]+)`)
			m := re.FindStringSubmatch(version)

			for i, name := range re.SubexpNames() {
				if len(m) > 0 && name == "version" {
					bundlerVersion = m[i]
				}
			}
		}
	}

	_, err = os.Stat("node_modules")
	vars["node"] = !os.IsNotExist(err)

	_, err = os.Stat("yarn.lock")
	vars["yarn"] = !os.IsNotExist(err)

	vars["rubyVersion"] = rubyVersion
	vars["bundlerVersion"] = bundlerVersion
	vars["nodeVersion"] = nodeVersion
	s.Files = templatesExecute("templates/bridgetown", vars)

	s.SkipDeploy = true
	s.DeployDocs = `
Your Bridgetown app is prepared for deployment.

If you need custom packages installed, or have problems with your deployment
build, you may need to edit the Dockerfile for app-specific changes. If you
need help, please post on https://community.fly.io.

Now: run 'fly deploy' to deploy your Bridgetown app.
`

	return s, nil
}
