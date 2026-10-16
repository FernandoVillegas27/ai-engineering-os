package detector

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Technology struct {
	Name     string   `json:"name" yaml:"name"`
	Version  string   `json:"version,omitempty" yaml:"version,omitempty"`
	Evidence []string `json:"evidence,omitempty" yaml:"evidence,omitempty"`
}

type Detection struct {
	Frontend       []Technology `json:"frontend" yaml:"frontend"`
	Backend        []Technology `json:"backend" yaml:"backend"`
	Database       []Technology `json:"database" yaml:"database"`
	Testing        []Technology `json:"testing" yaml:"testing"`
	Build          []Technology `json:"build" yaml:"build"`
	Runtime        []Technology `json:"runtime" yaml:"runtime"`
	Infrastructure []Technology `json:"infrastructure" yaml:"infrastructure"`
}

func Inspect(root string) (Detection, error) {
	if root == "" {
		return Detection{}, errors.New("project root is required")
	}

	info, err := os.Stat(root)
	if err != nil {
		return Detection{}, err
	}
	if !info.IsDir() {
		return Detection{}, errors.New("project root must be a directory")
	}

	var result Detection

	if err := inspectPackageJSON(root, &result); err != nil {
		return Detection{}, err
	}
	if err := inspectPom(root, &result); err != nil {
		return Detection{}, err
	}
	if err := inspectGoMod(root, &result); err != nil {
		return Detection{}, err
	}
	if err := inspectPyProject(root, &result); err != nil {
		return Detection{}, err
	}

	inspectFileSignals(root, &result)

	return result, nil
}

type packageJSON struct {
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
	Engines         map[string]string `json:"engines"`
}

func inspectPackageJSON(root string, result *Detection) error {
	path := filepath.Join(root, "package.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	var pkg packageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return err
	}

	deps := mergeDependencies(pkg.Dependencies, pkg.DevDependencies)

	if version, ok := deps["@angular/core"]; ok {
		addTechnology(&result.Frontend, Technology{Name: "angular", Version: majorVersion(version), Evidence: []string{"package.json"}})
	}
	if version, ok := deps["react"]; ok {
		addTechnology(&result.Frontend, Technology{Name: "react", Version: majorVersion(version), Evidence: []string{"package.json"}})
	}
	if version, ok := deps["next"]; ok {
		addTechnology(&result.Frontend, Technology{Name: "nextjs", Version: majorVersion(version), Evidence: []string{"package.json"}})
	}
	if version, ok := deps["typescript"]; ok {
		addTechnology(&result.Frontend, Technology{Name: "typescript", Version: majorVersion(version), Evidence: []string{"package.json"}})
	}
	if _, ok := deps["jest"]; ok {
		addTechnology(&result.Testing, Technology{Name: "jest", Evidence: []string{"package.json"}})
	}
	if version, ok := deps["@playwright/test"]; ok {
		addTechnology(&result.Testing, Technology{Name: "playwright", Version: majorVersion(version), Evidence: []string{"package.json"}})
	}
	if version, ok := pkg.Engines["node"]; ok {
		addTechnology(&result.Runtime, Technology{Name: "node", Version: majorVersion(version), Evidence: []string{"package.json"}})
	}

	return nil
}

type pomProject struct {
	Parent struct {
		ArtifactID string `xml:"artifactId"`
		Version    string `xml:"version"`
	} `xml:"parent"`
	Properties struct {
		JavaVersion           string `xml:"java.version"`
		MavenCompilerRelease  string `xml:"maven.compiler.release"`
		MavenCompilerSource   string `xml:"maven.compiler.source"`
		SpringBootVersion     string `xml:"spring-boot.version"`
	} `xml:"properties"`
	Dependencies []pomDependency `xml:"dependencies>dependency"`
}

type pomDependency struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
}

func inspectPom(root string, result *Detection) error {
	path := filepath.Join(root, "pom.xml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	var pom pomProject
	if err := xml.Unmarshal(data, &pom); err != nil {
		return err
	}

	addTechnology(&result.Build, Technology{Name: "maven", Evidence: []string{"pom.xml"}})

	javaVersion := firstNonEmpty(
		pom.Properties.JavaVersion,
		pom.Properties.MavenCompilerRelease,
		pom.Properties.MavenCompilerSource,
	)
	if javaVersion != "" {
		addTechnology(&result.Backend, Technology{Name: "java", Version: majorVersion(javaVersion), Evidence: []string{"pom.xml"}})
		addTechnology(&result.Runtime, Technology{Name: "java", Version: majorVersion(javaVersion), Evidence: []string{"pom.xml"}})
	}

	springVersion := ""
	if pom.Parent.ArtifactID == "spring-boot-starter-parent" {
		springVersion = pom.Parent.Version
	}
	if springVersion == "" {
		springVersion = pom.Properties.SpringBootVersion
	}

	springDetected := springVersion != ""
	for _, dep := range pom.Dependencies {
		if strings.HasPrefix(dep.ArtifactID, "spring-boot-") {
			springDetected = true
			if springVersion == "" {
				springVersion = dep.Version
			}
		}
		if strings.Contains(strings.ToLower(dep.ArtifactID), "junit") ||
			strings.Contains(strings.ToLower(dep.GroupID), "junit") {
			addTechnology(&result.Testing, Technology{Name: "junit", Version: majorVersion(dep.Version), Evidence: []string{"pom.xml"}})
		}
	}

	if springDetected {
		addTechnology(&result.Backend, Technology{Name: "spring-boot", Version: majorVersion(springVersion), Evidence: []string{"pom.xml"}})
	}

	return nil
}

func inspectGoMod(root string, result *Detection) error {
	path := filepath.Join(root, "go.mod")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	re := regexp.MustCompile(`(?m)^go\s+([0-9]+(?:\.[0-9]+)*)`)
	version := ""
	if match := re.FindStringSubmatch(string(data)); len(match) == 2 {
		version = match[1]
	}

	addTechnology(&result.Backend, Technology{Name: "go", Version: version, Evidence: []string{"go.mod"}})
	addTechnology(&result.Runtime, Technology{Name: "go", Version: version, Evidence: []string{"go.mod"}})

	return nil
}

func inspectPyProject(root string, result *Detection) error {
	path := filepath.Join(root, "pyproject.toml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	content := strings.ToLower(string(data))
	addTechnology(&result.Backend, Technology{Name: "python", Evidence: []string{"pyproject.toml"}})
	addTechnology(&result.Runtime, Technology{Name: "python", Evidence: []string{"pyproject.toml"}})

	if strings.Contains(content, "django") {
		addTechnology(&result.Backend, Technology{Name: "django", Evidence: []string{"pyproject.toml"}})
	}
	if strings.Contains(content, "pytest") {
		addTechnology(&result.Testing, Technology{Name: "pytest", Evidence: []string{"pyproject.toml"}})
	}

	return nil
}

func inspectFileSignals(root string, result *Detection) {
	if fileExists(filepath.Join(root, "angular.json")) {
		addTechnology(&result.Frontend, Technology{Name: "angular", Evidence: []string{"angular.json"}})
	}
	if fileExists(filepath.Join(root, "package-lock.json")) {
		addTechnology(&result.Build, Technology{Name: "npm", Evidence: []string{"package-lock.json"}})
	}
	if fileExists(filepath.Join(root, "pnpm-lock.yaml")) {
		addTechnology(&result.Build, Technology{Name: "pnpm", Evidence: []string{"pnpm-lock.yaml"}})
	}
	if fileExists(filepath.Join(root, "yarn.lock")) {
		addTechnology(&result.Build, Technology{Name: "yarn", Evidence: []string{"yarn.lock"}})
	}
	if fileExists(filepath.Join(root, "Dockerfile")) {
		addTechnology(&result.Infrastructure, Technology{Name: "docker", Evidence: []string{"Dockerfile"}})
	}
	if fileExists(filepath.Join(root, "docker-compose.yml")) {
		addTechnology(&result.Infrastructure, Technology{Name: "docker-compose", Evidence: []string{"docker-compose.yml"}})
	}
	if fileExists(filepath.Join(root, "docker-compose.yaml")) {
		addTechnology(&result.Infrastructure, Technology{Name: "docker-compose", Evidence: []string{"docker-compose.yaml"}})
	}
}

func mergeDependencies(parts ...map[string]string) map[string]string {
	result := make(map[string]string)
	for _, part := range parts {
		for name, version := range part {
			result[name] = version
		}
	}
	return result
}

func addTechnology(target *[]Technology, candidate Technology) {
	for i := range *target {
		if (*target)[i].Name != candidate.Name {
			continue
		}

		if (*target)[i].Version == "" && candidate.Version != "" {
			(*target)[i].Version = candidate.Version
		}
		(*target)[i].Evidence = mergeEvidence((*target)[i].Evidence, candidate.Evidence)
		return
	}

	candidate.Evidence = mergeEvidence(nil, candidate.Evidence)
	*target = append(*target, candidate)
}

func mergeEvidence(current, incoming []string) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(current)+len(incoming))

	for _, value := range append(current, incoming...) {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}

	return out
}

func majorVersion(value string) string {
	re := regexp.MustCompile(`[0-9]+`)
	return re.FindString(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
