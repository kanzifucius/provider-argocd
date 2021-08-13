package projects

import (
	"context"
	"github.com/argoproj/argo-cd/v2/pkg/apiclient/project"
	"strings"

	"github.com/argoproj/argo-cd/v2/pkg/apiclient"
	"github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"

	"google.golang.org/grpc"
)

const (
	errorProjectNotFound = "code = NotFound desc = project"
)

// ProjectServiceClient wraps the functions to connect to argocd repositories
type ProjectServiceClient interface {
	// Get returns a repository or its credentials
	Get(ctx context.Context, in *project.ProjectQuery, opts ...grpc.CallOption) (*v1alpha1.AppProject, error)
	// List  gets a list of all configured repositories
	List(ctx context.Context, in *project.ProjectQuery, opts ...grpc.CallOption) (*v1alpha1.AppProjectList, error)
	// Create Create creates a project
	Create(ctx context.Context, in *project.ProjectCreateRequest, opts ...grpc.CallOption) (*v1alpha1.AppProject, error)
	// Update Update updates a project
	Update(ctx context.Context, in *project.ProjectUpdateRequest, opts ...grpc.CallOption) (*v1alpha1.AppProject, error)
	// Delete Delete deletes a project from argo
	Delete(ctx context.Context, in *project.ProjectQuery, opts ...grpc.CallOption) (*project.EmptyResponse, error)
}

// NewProjectServiceClient creates a new API client from a set of config options, or fails fatally if the new client creation fails.
func NewProjectServiceClient(clientOpts *apiclient.ClientOptions) project.ProjectServiceClient {
	_, repoIf := apiclient.NewClientOrDie(clientOpts).NewProjectClientOrDie()
	return repoIf
}

// IsErrorProjectNotFound helper function to test for errorRepositoryNotFound error.
func IsErrorProjectNotFound(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), errorProjectNotFound)
}
