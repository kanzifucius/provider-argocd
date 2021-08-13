/*
Copyright 2021 The Crossplane Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package projects

import (
	"context"

	"github.com/crossplane-contrib/provider-argocd/pkg/clients/projects"

	"github.com/argoproj/argo-cd/v2/pkg/apiclient"
	"github.com/argoproj/argo-cd/v2/pkg/apiclient/project"
	argocdv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	"github.com/crossplane-contrib/provider-argocd/apis/projects/v1alpha1"
	"github.com/crossplane-contrib/provider-argocd/pkg/clients"
)

const (
	errNotProject       = "managed resource is not a Argocd project custom resource"
	errGetFailed        = "cannot get Argocd project"
	errKubeUpdateFailed = "cannot update Argocd project custom resource"
	errCreateFailed     = "cannot create Argocd project"
	errUpdateFailed     = "cannot update Argocd project"
	errDeleteFailed     = "cannot delete Argocd project"
)

// SetupProject adds a controller that reconciles repositories.
func SetupProject(mgr ctrl.Manager, l logging.Logger) error {
	name := managed.ControllerName(v1alpha1.ProjectKind)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		For(&v1alpha1.Project{}).
		Complete(managed.NewReconciler(mgr,
			resource.ManagedKind(v1alpha1.ProjectGroupVersionKind),
			managed.WithExternalConnecter(&connector{kube: mgr.GetClient(), newArgocdClientFn: projects.NewProjectServiceClient}),
			managed.WithInitializers(managed.NewDefaultProviderConfig(mgr.GetClient())),
			managed.WithLogger(l.WithValues("controller", name)),
			managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name)))))
}

type connector struct {
	kube              client.Client
	newArgocdClientFn func(clientOpts *apiclient.ClientOptions) project.ProjectServiceClient
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.Project)
	if !ok {
		return nil, errors.New(errNotProject)
	}
	cfg, err := clients.GetConfig(ctx, c.kube, cr)
	if err != nil {
		return nil, err
	}
	return &external{kube: c.kube, client: c.newArgocdClientFn(cfg)}, nil
}

type external struct {
	kube   client.Client
	client projects.ProjectServiceClient
}

func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.Project)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotProject)
	}

	if meta.GetExternalName(cr) == "" {
		return managed.ExternalObservation{
			ResourceExists: false,
		}, nil
	}

	projectQuery := project.ProjectQuery{
		Name: meta.GetExternalName(cr),
	}

	appProject, err := e.client.Get(ctx, &projectQuery)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetFailed)
	}

	current := cr.Spec.ForProvider.DeepCopy()
	lateInitializeProject(&cr.Spec.ForProvider, appProject)

	cr.Status.AtProvider = generateProjectObservation(appProject)
	cr.Status.SetConditions(xpv1.Available())

	return managed.ExternalObservation{
		ResourceExists:          true,
		ResourceUpToDate:        isProjectUpToDate(&cr.Spec.ForProvider, appProject),
		ResourceLateInitialized: !cmp.Equal(current, &cr.Spec.ForProvider),
	}, nil
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.Project)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotProject)
	}

	appCreateRequest := generateCreateProjectOptions(&cr.Spec.ForProvider)

	_, err := e.client.Create(ctx, appCreateRequest)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateFailed)
	}

	meta.SetExternalName(cr, *cr.Spec.ForProvider.Name)

	return managed.ExternalCreation{
		ExternalNameAssigned: true,
	}, errors.Wrap(nil, errKubeUpdateFailed)
}

func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.Project)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotProject)
	}

	appProjectUpdateRequest := generateUpdateProjectOptions(&cr.Spec.ForProvider)

	_, err := e.client.Update(ctx, appProjectUpdateRequest)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateFailed)
	}

	return managed.ExternalUpdate{}, nil
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) error {
	cr, ok := mg.(*v1alpha1.Project)
	if !ok {
		return errors.New(errNotProject)
	}
	repoQuery := project.ProjectQuery{
		Name: meta.GetExternalName(cr),
	}

	_, err := e.client.Delete(ctx, &repoQuery)

	return errors.Wrap(err, errDeleteFailed)
}

func lateInitializeProject(p *v1alpha1.ProjectParameters, r *argocdv1alpha1.AppProject) { // nolint:gocyclo
	if r == nil {
		return
	}

	p.Name = clients.LateInitializeStringPtr(p.Name, r.Name)
}

func generateProjectObservation(p *argocdv1alpha1.AppProject) v1alpha1.ProjectObservation {
	if p == nil {
		return v1alpha1.ProjectObservation{}
	}
	o := v1alpha1.ProjectObservation{}
	return o
}

func generateCreateProjectOptions(p *v1alpha1.ProjectParameters) *project.ProjectCreateRequest {
	proj := &argocdv1alpha1.AppProject{}
	//if p.Username != nil {
	//	repo.Username = *p.Username
	//}
	//if p.Insecure != nil {
	//	repo.Insecure = *p.Insecure
	//}
	//if p.EnableLFS != nil {
	//	repo.EnableLFS = *p.EnableLFS
	//}
	//if p.Type != nil {
	//	repo.Type = *p.Type
	//}
	//if p.Name != nil {
	//	repo.Name = *p.Name
	//}
	//if p.EnableOCI != nil {
	//	repo.EnableOCI = *p.EnableOCI
	//}
	//if p.InheritedCreds != nil {
	//	repo.InheritedCreds = *p.InheritedCreds
	//}
	//if p.GithubAppID != nil {
	//	repo.GithubAppId = *p.GithubAppID
	//}
	//if p.GithubAppInstallationID != nil {
	//	repo.GithubAppInstallationId = *p.GithubAppInstallationID
	//}
	//if p.GitHubAppEnterpriseBaseURL != nil {
	//	repo.GitHubAppEnterpriseBaseURL = *p.GitHubAppEnterpriseBaseURL
	//}

	projCreateRequest := &project.ProjectCreateRequest{
		Project: proj,
		Upsert:  false,
	}

	return projCreateRequest
}

func generateUpdateProjectOptions(p *v1alpha1.ProjectParameters) *project.ProjectUpdateRequest {
	proj := &argocdv1alpha1.AppProject{}

	//TODO implement
	//
	//if p.Username != nil {
	//	repo.Username = *p.Username
	//}
	//if p.Type != nil {
	//	repo.Type = *p.Type
	//}
	//if p.Name != nil {
	//	repo.Name = *p.Name
	//}
	//if p.GithubAppID != nil {
	//	repo.GithubAppId = *p.GithubAppID
	//}
	//if p.GithubAppInstallationID != nil {
	//	repo.GithubAppInstallationId = *p.GithubAppInstallationID
	//}
	//if p.GitHubAppEnterpriseBaseURL != nil {
	//	repo.GitHubAppEnterpriseBaseURL = *p.GitHubAppEnterpriseBaseURL
	//}

	o := &project.ProjectUpdateRequest{
		Project: proj,
	}
	return o
}

func isProjectUpToDate(p *v1alpha1.ProjectParameters, app *argocdv1alpha1.AppProject) bool {

	//TODO implement comparison
	//if !cmp.Equal(p.Username, clients.StringToPtr(app.Username)) {
	//	return false
	//}
	//if !clients.IsBoolEqualToBoolPtr(p.Insecure, app.Insecure) {
	//	return false
	//}

	return true
}
