package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/kelseyhightower/envconfig"
	uuid "github.com/satori/go.uuid"
	"github.com/willabides/azurefuncs/azdeployer"
	"gopkg.in/yaml.v2"
)

type config struct {
	TenantID           string `envconfig:"AZURE_TENANT_ID"`
	ClientID           string `envconfig:"AZURE_CLIENT_ID"`
	ClientSecret       string `envconfig:"AZURE_CLIENT_SECRET"`
	SubscriptionID     string `envconfig:"AZURE_SUBSCRIPTION_ID"`
	StorageAccountKey  string `envconfig:"AZURE_STORAGE_ACCOUNT_KEY"`
	StorageAccountName string `envconfig:"AZURE_STORAGE_ACCOUNT_NAME"`
}

func main() {
	var cfg config
	ctx := context.Background()
	err := envconfig.Process("azure", &cfg)
	if err != nil {
		log.Fatal(err)
	}

	var sourcePath string
	flag.StringVar(&sourcePath, "source-path", ".", "path to the source root")

	var outputPath string
	flag.StringVar(&outputPath, "output", "./dist/bundle.zip", "where to write output")

	var blobContainer string
	flag.StringVar(&blobContainer, "container", "function-releases", "blob container for the bundle")

	var resourceGroup string
	flag.StringVar(&resourceGroup, "rg", "", "resource group to deploy to")

	var appName string
	flag.StringVar(&appName, "app", "", "app name")

	flag.Parse()

	if appName == "" {
		log.Fatal("-app is required")
	}
	if resourceGroup == "" {
		log.Fatal("-rg is required")
	}

	funcPath := filepath.Join(flag.Arg(0), "functions.yml")
	err = buildArchive(funcPath, outputPath)
	if err != nil {
		log.Fatalf("err: %v", err)
	}
	zipReader, err := os.Open(outputPath) //nolint:gosec // checked
	if err != nil {
		log.Fatalf("err: %v", err)
	}
	storageClient, err := azdeployer.NewBlobStorageClient(cfg.StorageAccountName, cfg.StorageAccountKey)
	if err != nil {
		log.Fatalf("err: %v", err)
	}
	blobName := fmt.Sprintf("bundle-%s.zip", uuid.NewV4().String())
	err = storageClient.UploadBlob(ctx, blobContainer, blobName, zipReader)
	if err != nil {
		log.Fatalf("err: %v", err)
	}
	signBlobURL, err := storageClient.SignBlobURL(blobContainer, blobName)
	if err != nil {
		log.Fatalf("err: %v", err)
	}
	credConfig := auth.NewClientCredentialsConfig(cfg.ClientID, cfg.ClientSecret, cfg.TenantID)
	ac := azdeployer.NewAppsClient(cfg.SubscriptionID, credConfig)
	err = ac.UpdateAppSettings(ctx, resourceGroup, appName, map[string]string{
		"WEBSITE_RUN_FROM_PACKAGE": signBlobURL,
	})
	if err != nil {
		log.Fatalf("err: %v", err)
	}
	err = ac.SyncFunctionTriggers(ctx, resourceGroup, appName)
	if err != nil {
		log.Fatalf("err: %v", err)
	}
}

func buildArchive(funcPath, outputPath string) error {
	builder := new(azdeployer.ArchiveBuilder)
	functionsFile, err := ioutil.ReadFile(funcPath) //nolint:gosec // checked
	if err != nil {
		return fmt.Errorf("couldn't read file %s: %v", funcPath, err)
	}
	err = yaml.Unmarshal(functionsFile, builder)
	if err != nil {
		return fmt.Errorf("couldn't unmarshal %s: %v", funcPath, err)
	}
	err = os.MkdirAll(filepath.Dir(outputPath), 0o700)
	if err != nil {
		return fmt.Errorf("couldn't create directory %s: %v", filepath.Dir(outputPath), err)
	}
	zipFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("couldn't create file %s: %v", outputPath, err)
	}
	err = builder.BuildArchive(zipFile)
	if err != nil {
		return fmt.Errorf("couldn't build archive: %v", err)
	}
	err = zipFile.Close()
	if err != nil {
		return fmt.Errorf("couldn't finalize zip file: %v", err)
	}
	return nil
}
