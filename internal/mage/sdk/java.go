package sdk

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"dagger.io/dagger"
	"github.com/dagger/dagger/internal/mage/util"
	"github.com/magefile/mage/mg"
)

const (
	javaSDKPath = "sdk/java"
	javaSDKVersionPomPath = javaSDKPath + "/pom.xml"

	javaVersion = "17"
	mavenVersion = "3.9"
)

var _ SDK = Java{}

type Java mg.Namespace

// Lint lints the Java SDK
func (Java) Lint(ctx context.Context) error {
	c, err := dagger.Connect(ctx, dagger.WithLogOutput(os.Stderr))
	if err != nil {
		return err
	}
	defer c.Close()

	c = c.Pipeline("sdk").Pipeline("java").Pipeline("lint")

	_, err = javaBase(c).
		WithExec([]string{"mvn", "fmt:check"}).
		Sync(ctx)

	return err;
}

// Test tests the Java SDK
func (Java) Test(ctx context.Context) error {
	c, err := dagger.Connect(ctx, dagger.WithLogOutput(os.Stderr))
	if err != nil {
		return err
	}
	defer c.Close()

	c = c.Pipeline("sdk").Pipeline("java").Pipeline("test")

	devEngine, endpoint, err := util.CIDevEngineContainerAndEndpoint(
		ctx,
		c.Pipeline("dev-engine"),
		util.DevEngineOpts{Name: "sdk-java-test"},
	)
	if err != nil {
		return err
	}

	cliBinPath := "/.dagger-cli"

	_, err = javaBase(c).
		WithServiceBinding("dagger-engine", devEngine).
		WithEnvVariable("_EXPERIMENTAL_DAGGER_RUNNER_HOST", endpoint).
		WithMountedFile(cliBinPath, util.DaggerBinary(c)).
		WithEnvVariable("_EXPERIMENTAL_DAGGER_CLI_BIN", cliBinPath).
		WithExec([]string{"mvn", "clean", "verify"}).
		Sync(ctx)
	if err != nil {
		return err
	}
	return nil
}

// Generate re-generates the SDK API
func (Java) Generate(ctx context.Context) error {
	c, err := dagger.Connect(ctx, dagger.WithLogOutput(os.Stderr))
	if err != nil {
		return err
	}
	defer c.Close()

	c = c.Pipeline("sdk").Pipeline("java").Pipeline("generate")

	devEngine, endpoint, err := util.CIDevEngineContainerAndEndpoint(
		ctx,
		c.Pipeline("dev-engine"),
		util.DevEngineOpts{Name: "sdk-java-generate"},
	)
	if err != nil {
		return err
	}

	cliBinPath := "/.dagger-cli"

	_, err = javaBase(c).
		WithServiceBinding("dagger-engine", devEngine).
		WithEnvVariable("_EXPERIMENTAL_DAGGER_RUNNER_HOST", endpoint).
		WithMountedFile(cliBinPath, util.DaggerBinary(c)).
		WithEnvVariable("_EXPERIMENTAL_DAGGER_CLI_BIN", cliBinPath).
		WithExec([]string{"mvn", "clean", "package", "-Pjavadoc"}).
		Sync(ctx)

	return err
}

// Publish publishes the Java SDK
func (Java) Publish(ctx context.Context, tag string) error {
	c, err := dagger.Connect(ctx, dagger.WithLogOutput(os.Stderr))
	if err != nil {
		return err
	}
	defer c.Close()

	var (
		version = strings.TrimPrefix(tag, "sdk/java/v")
		dryRun  = os.Getenv("MVN_DEPLOY_DRY_RUN")
	)

	skipDeploy := "true" // FIXME: Always set to true as long as the maven central deployment is not configured
	if dryRun != "" {
		skipDeploy = "true"
	}

	c = c.Pipeline("sdk").Pipeline("java").Pipeline("deploy")

	_, err = javaBase(c).
		WithExec([]string{"apt-get", "update"}).
		WithExec([]string{"apt-get", "-y", "install", "gpg"}).
		WithExec([]string{"mvn", "versions:set", fmt.Sprintf("-DnewVersion=%s", version)}).
		WithExec([]string{"mvn", "clean", "deploy", "-Prelease", fmt.Sprintf("-Dmaven.deploy.skip=%s", skipDeploy)}).
		WithExec([]string{"find", ".", "-name", "*.jar"}).
		Sync(ctx)
	return err
}

// Bump the Java SDK's Engine dependency
func (Java) Bump(ctx context.Context, engineVersion string) error {
	contents, err := os.ReadFile(javaSDKVersionPomPath)
	if err != nil {
		return err
	}

	newVersion := fmt.Sprintf(`<daggerengine.version>%s</daggerengine.version>`, strings.TrimPrefix(engineVersion, "v"))

	versionRe, err := regexp.Compile(`<daggerengine.version>([0-9\.-a-zA-Z]+)</daggerengine.version>`)
	if err != nil {
		return err
	}
	newContents := versionRe.ReplaceAll(contents, []byte(newVersion))
	return os.WriteFile(javaSDKVersionPomPath, newContents, 0o600)
}

func javaBase(c *dagger.Client) *dagger.Container {
	const appDir = "sdk/java"
	src := c.Directory().WithDirectory("/", util.Repository(c).Directory(appDir))

	mountPath := fmt.Sprintf("/%s", appDir)

	mavenCache := c.CacheVolume("maven-cache")

	return (c.Container().
		From(fmt.Sprintf("maven:%s-eclipse-temurin-%s", mavenVersion, javaVersion)).
		WithWorkdir(mountPath).
		WithDirectory(mountPath, src)).
		WithMountedCache("/root/.m2", mavenCache)
}
