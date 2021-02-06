package azdeployer

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/web/mgmt/2020-06-01/web"
	"github.com/Azure/azure-storage-blob-go/azblob"
	"github.com/Azure/go-autorest/autorest/azure/auth"
)

// BlobStorageClient is a client for azure blob storage
type BlobStorageClient struct {
	StorageAccountCredential *azblob.SharedKeyCredential
	StorageAccountName       string
}

// NewBlobStorageClient builds a BlobStorageClient
func NewBlobStorageClient(accountName, accountKey string) (*BlobStorageClient, error) {
	credential, err := azblob.NewSharedKeyCredential(accountName, accountKey)
	if err != nil {
		return nil, err
	}
	return &BlobStorageClient{
		StorageAccountCredential: credential,
		StorageAccountName:       accountName,
	}, nil
}

func (d *BlobStorageClient) blobServiceURL() azblob.ServiceURL {
	return azblob.NewServiceURL(
		url.URL{
			Scheme: "https",
			Host:   d.StorageAccountName + ".blob.core.windows.net",
		},
		azblob.NewPipeline(d.StorageAccountCredential, azblob.PipelineOptions{}),
	)
}

func (d *BlobStorageClient) containerURL(containerName string) azblob.ContainerURL {
	return d.blobServiceURL().NewContainerURL(containerName)
}

func (d *BlobStorageClient) blockBlobURL(containerName, blobName string) azblob.BlockBlobURL {
	return d.containerURL(containerName).NewBlockBlobURL(blobName)
}

// SignBlobURL returns a signed url with read access that lasts a thousand years
func (d *BlobStorageClient) SignBlobURL(containerName, blobName string) (string, error) {
	sasQueryParams, err := azblob.BlobSASSignatureValues{
		Protocol:      azblob.SASProtocolHTTPS, // Users MUST use HTTPS (not HTTP)
		ContainerName: containerName,
		BlobName:      blobName,
		ExpiryTime:    time.Now().UTC().AddDate(1000, 0, 0),
		Permissions:   azblob.BlobSASPermissions{Read: true}.String(),
	}.NewSASQueryParameters(d.StorageAccountCredential)
	if err != nil {
		return "", err
	}
	qp := sasQueryParams.Encode()
	signedURL := fmt.Sprintf(`https://%s.blob.core.windows.net/%s/%s?%s`,
		d.StorageAccountName, containerName, blobName, qp)
	return signedURL, nil
}

// UploadBlob uploads a blob
func (d *BlobStorageClient) UploadBlob(ctx context.Context, containerName, blobName string, content io.ReadSeeker) error {
	_, err := d.blockBlobURL(containerName, blobName).Upload(ctx,
		content,
		azblob.BlobHTTPHeaders{ContentType: `application/octet-stream`},
		azblob.Metadata{},
		azblob.BlobAccessConditions{},
		azblob.DefaultAccessTier,
		nil,
		azblob.ClientProvidedKeyOptions{},
	)
	return err
}

// AppsClient is a client for azure function apps
type AppsClient struct {
	subscriptionID string
	authConfig     auth.AuthorizerConfig
}

// NewAppsClient returns a new AppsClient
func NewAppsClient(subscriptionID string, authConfig auth.AuthorizerConfig) *AppsClient {
	return &AppsClient{
		subscriptionID: subscriptionID,
		authConfig:     authConfig,
	}
}

func (d *AppsClient) client() (*web.AppsClient, error) {
	var err error
	client := web.NewAppsClient(d.subscriptionID)
	client.Authorizer, err = d.authConfig.Authorizer()
	if err != nil {
		return nil, err
	}
	return &client, nil
}

func (d *AppsClient) servicePlansClient() (*web.AppServicePlansClient, error) {
	var err error
	client := web.NewAppServicePlansClient(d.subscriptionID)
	client.Authorizer, err = d.authConfig.Authorizer()
	if err != nil {
		return nil, err
	}
	return &client, nil
}

// UpdateAppSettings updates app settings with newSettings. Leaves other settings along.
func (d *AppsClient) UpdateAppSettings(ctx context.Context, resourceGroup, appName string, newSettings map[string]string) error {
	client, err := d.client()
	if err != nil {
		return err
	}
	settings, err := client.ListApplicationSettings(ctx, resourceGroup, appName)
	if err != nil {
		return err
	}
	for k, v := range newSettings {
		v := v
		settings.Properties[k] = &v
	}
	_, err = client.UpdateApplicationSettings(ctx, resourceGroup, appName, settings)
	return err
}

// CreateOrUpdateServicePlan creates or updates a dynamic linux AppServicePlan
func (d *AppsClient) CreateOrUpdateServicePlan(ctx context.Context, resourceGroup, planName, location string) error {
	client, err := d.servicePlansClient()
	if err != nil {
		return err
	}
	_, err = client.CreateOrUpdate(ctx, resourceGroup, planName, web.AppServicePlan{
		Location: &location,
		Kind:     stringPtr("functionapp"),
		Type:     stringPtr("Microsoft.Web/serverfarms"),
		AppServicePlanProperties: &web.AppServicePlanProperties{
			Reserved: boolPtr(true), // Reserved means linux for some reason
		},
		Sku: &web.SkuDescription{
			Name: stringPtr("Y1"),
		},
	})
	return err
}

// GetLocationPlan returns a suitable service plan in a given location
func (d *AppsClient) GetLocationPlan(ctx context.Context, resourceGroup, location string) (name string, found bool, err error) {
	client, err := d.servicePlansClient()
	if err != nil {
		return "", false, err
	}

	plans, err := client.ListByResourceGroup(ctx, resourceGroup)
	if err != nil {
		return "", false, err
	}

	for {
		foundPlan := findLocationPlan(plans.Values(), location)
		if foundPlan != "" {
			return foundPlan, true, nil
		}
		if !plans.NotDone() {
			break
		}
		err = plans.NextWithContext(ctx)
		if err != nil {
			return "", false, err
		}
	}
	return "", false, nil
}

func findLocationPlan(plans []web.AppServicePlan, location string) string {
	for _, plan := range plans {
		if plan.Location == nil ||
			plan.Kind == nil ||
			plan.Type == nil ||
			plan.Sku == nil ||
			plan.Sku.Name == nil ||
			plan.Reserved == nil ||
			plan.Name == nil {
			continue
		}
		if *plan.Location != location {
			continue
		}
		if *plan.Sku.Name != "Y1" {
			continue
		}
		// Reserved means linux for some reason
		if !*plan.Reserved {
			continue
		}
		if *plan.Kind != "functionapp" {
			continue
		}
		if *plan.Type != "Microsoft.Web/serverfarms" {
			continue
		}
		return *plan.Name
	}
	return ""
}

func stringPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}

func (d *AppsClient) serverFarmID(resourceGroup, planName string) string {
	return fmt.Sprintf(`/subscriptions/%s/resourceGroups/%s/providers/Microsoft.web/serverfarms/%s`, d.subscriptionID, resourceGroup, planName)
}

// CreateFunctionApp creates an function app
func (d *AppsClient) CreateFunctionApp(ctx context.Context, resourceGroup, appName, contentURL, location, servicePlan string) error {
	client, err := d.client()
	if err != nil {
		return err
	}
	kind := `functionapp,linux`
	if servicePlan == "" {
		var foundPlan bool
		servicePlan, foundPlan, err = d.GetLocationPlan(ctx, resourceGroup, location)
		if err != nil {
			return err
		}
		if !foundPlan {
			return fmt.Errorf("couldn't find a valid service plan")
		}
	}

	serverFarmID := d.serverFarmID(resourceGroup, servicePlan)
	_, err = client.CreateOrUpdate(ctx, resourceGroup, appName, web.Site{
		Kind:     &kind,
		Location: &location,
		SiteProperties: &web.SiteProperties{
			ServerFarmID: &serverFarmID,
		},
	})
	if err != nil {
		return err
	}
	err = d.UpdateAppSettings(ctx, resourceGroup, appName, map[string]string{
		`WEBSITE_RUN_FROM_PACKAGE`:    contentURL,
		`WEBSITE_MOUNT_ENABLED`:       `1`,
		`FUNCTIONS_EXTENSION_VERSION`: `~3`,
		`FUNCTIONS_WORKER_RUNTIME`:    `custom`,
	})
	return err
}

// SyncFunctionTriggers syncs function triggers. You need to do this after updating WEBSITE_RUN_FROM_PACKAGE
func (d *AppsClient) SyncFunctionTriggers(ctx context.Context, resourceGroup, appName string) error {
	client, err := d.client()
	if err != nil {
		return err
	}
	_, err = client.SyncFunctionTriggers(ctx, resourceGroup, appName)
	return err
}
