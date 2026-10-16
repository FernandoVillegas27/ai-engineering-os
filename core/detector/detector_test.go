package detector

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInspectAngularSpringProject(t *testing.T) {
	root := t.TempDir()

	writeTestFile(t, root, "package.json", `{
		"dependencies": {
			"@angular/core": "^20.2.0"
		},
		"devDependencies": {
			"typescript": "^5.9.0",
			"@playwright/test": "^1.55.0"
		},
		"engines": {
			"node": ">=22"
		}
	}`)
	writeTestFile(t, root, "angular.json", "{}")
	writeTestFile(t, root, "package-lock.json", "{}")
	writeTestFile(t, root, "Dockerfile", "FROM eclipse-temurin:21")
	writeTestFile(t, root, "pom.xml", `<project>
		<parent>
			<artifactId>spring-boot-starter-parent</artifactId>
			<version>3.5.0</version>
		</parent>
		<properties>
			<java.version>21</java.version>
		</properties>
		<dependencies>
			<dependency>
				<groupId>org.springframework.boot</groupId>
				<artifactId>spring-boot-starter-web</artifactId>
			</dependency>
			<dependency>
				<groupId>org.junit.jupiter</groupId>
				<artifactId>junit-jupiter</artifactId>
				<version>5.13.0</version>
			</dependency>
		</dependencies>
	</project>`)

	got, err := Inspect(root)
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}

	assertTechnology(t, got.Frontend, "angular", "20")
	assertTechnology(t, got.Frontend, "typescript", "5")
	assertTechnology(t, got.Backend, "java", "21")
	assertTechnology(t, got.Backend, "spring-boot", "3")
	assertTechnology(t, got.Testing, "junit", "5")
	assertTechnology(t, got.Testing, "playwright", "1")
	assertTechnology(t, got.Build, "maven", "")
	assertTechnology(t, got.Build, "npm", "")
	assertTechnology(t, got.Runtime, "node", "22")
	assertTechnology(t, got.Infrastructure, "docker", "")
}

func TestInspectGoProject(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.23\n")

	got, err := Inspect(root)
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}

	assertTechnology(t, got.Backend, "go", "1.23")
	assertTechnology(t, got.Runtime, "go", "1.23")
}

func TestInspectEmptyProject(t *testing.T) {
	got, err := Inspect(t.TempDir())
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}

	if len(got.Frontend) != 0 || len(got.Backend) != 0 || len(got.Testing) != 0 {
		t.Fatalf("expected no detections, got %#v", got)
	}
}

func writeTestFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func assertTechnology(t *testing.T, technologies []Technology, name, version string) {
	t.Helper()

	for _, technology := range technologies {
		if technology.Name != name {
			continue
		}
		if technology.Version != version {
			t.Fatalf("%s version = %q, want %q", name, technology.Version, version)
		}
		if len(technology.Evidence) == 0 {
			t.Fatalf("%s has no evidence", name)
		}
		return
	}

	t.Fatalf("technology %q not found in %#v", name, technologies)
}
