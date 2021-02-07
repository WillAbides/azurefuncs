package signstorage

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Azure/azure-storage-blob-go/azblob"
	"github.com/Azure/go-autorest/autorest/adal"
	uuid "github.com/satori/go.uuid"
)

// Handler returns a handler
func Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		got, err := doIt(req.Context())
		if err != nil {
			http.Error(w, "FOOFOO "+err.Error(), http.StatusInsufficientStorage)
			return
		}
		fmt.Fprintln(w, got)
	}
}

//nolint:gocritic // messy code
func doIt(ctx context.Context) (string, error) {
	accountName := "willabidesstorage"
	// Use the azure resource id of user assigned identity when creating the token.
	// identityResourceID := "/subscriptions/{subscriptionID}/resourceGroups/testGroup/providers/Microsoft.ManagedIdentity/userAssignedIdentities/test-identity"
	// resource := "https://resource"
	var applicationID, identityResourceID, resource string

	callbacks := func(token adal.Token) error { return nil }
	tokenCredentials, err := getOAuthToken(applicationID, identityResourceID, resource, callbacks)
	if err != nil {
		return "", err
	}
	p := azblob.NewPipeline(*tokenCredentials, azblob.PipelineOptions{})
	blobPrimaryURL, err := url.Parse("https://" + accountName + ".blob.core.windows.net/")
	if err != nil {
		return "", err
	}
	bsu := azblob.NewServiceURL(*blobPrimaryURL, p)
	containerName := "mycontainer"
	containerURL := bsu.NewContainerURL(containerName)
	blobName := fmt.Sprintf("test-create-%s", uuid.NewV4().String())
	blobURL := containerURL.NewBlockBlobURL(blobName)
	data := "Hello World!"
	uploadResp, err := blobURL.Upload(ctx, strings.NewReader(data),
		azblob.BlobHTTPHeaders{ContentType: "text/plain"},
		azblob.Metadata{},
		azblob.BlobAccessConditions{},
		azblob.DefaultAccessTier,
		nil,
		azblob.ClientProvidedKeyOptions{},
	)
	if err != nil {
		return "", err
	}
	if uploadResp.StatusCode() != 201 {
		return "", fmt.Errorf("not 201")
	}
	return blobURL.String(), nil

	//sigValues := azblob.BlobSASSignatureValues{
	//	Protocol:      azblob.SASProtocolHTTPS, // Users MUST use HTTPS (not HTTP)
	//	ContainerName: containerName,
	//	BlobName:      blobName,
	//	ExpiryTime:    time.Now().UTC().AddDate(1000, 0, 0),
	//	Permissions:   azblob.BlobSASPermissions{Read: true}.String(),
	//}
	//sigValues.NewSASQueryParameters(tokenCredentials)
}

func fetchMSIToken(applicationID, identityResourceID, resource string, callbacks ...adal.TokenRefreshCallback) (*adal.ServicePrincipalToken, error) {
	// Both application id and identityResourceId cannot be present at the same time.
	if applicationID != "" && identityResourceID != "" {
		return nil, fmt.Errorf("didn't expect applicationID and identityResourceID at same time")
	}

	// msiEndpoint is the well known endpoint for getting MSI authentications tokens
	// msiEndpoint := "http://169.254.169.254/metadata/identity/oauth2/token" for production Jobs
	msiEndpoint, err := adal.GetMSIVMEndpoint()
	if err != nil {
		return nil, err
	}

	var spt *adal.ServicePrincipalToken

	// both can be empty, systemAssignedMSI scenario
	if applicationID == "" && identityResourceID == "" {
		spt, err = adal.NewServicePrincipalTokenFromMSI(msiEndpoint, resource, callbacks...)
	}

	// msi login with clientID
	if applicationID != "" {
		spt, err = adal.NewServicePrincipalTokenFromMSIWithUserAssignedID(msiEndpoint, resource, applicationID, callbacks...)
	}

	// msi login with resourceID
	if identityResourceID != "" {
		spt, err = adal.NewServicePrincipalTokenFromMSIWithIdentityResourceID(msiEndpoint, resource, identityResourceID, callbacks...)
	}

	if err != nil {
		return nil, err
	}

	err = spt.Refresh()
	return spt, err
}

//nolint:gocritic // messy code
func getOAuthToken(applicationID, identityResourceID, resource string, callbacks ...adal.TokenRefreshCallback) (*azblob.TokenCredential, error) {
	spt, err := fetchMSIToken(applicationID, identityResourceID, resource, callbacks...)
	if err != nil {
		log.Fatal(err)
	}

	// Refresh obtains a fresh token for the Service Principal.
	err = spt.Refresh()
	if err != nil {
		log.Fatal(err)
	}

	tc := azblob.NewTokenCredential(spt.Token().AccessToken, func(tc azblob.TokenCredential) time.Duration {
		_ = spt.Refresh() //nolint:errcheck // copied this code from an example
		return time.Until(spt.Token().Expires())
	})

	return &tc, nil
}
