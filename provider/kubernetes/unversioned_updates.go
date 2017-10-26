package kubernetes

import (
	"fmt"

	"github.com/rusenask/keel/types"
	"github.com/rusenask/keel/util/image"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"

	log "github.com/Sirupsen/logrus"
)

// func (p *Provider) checkUnversionedDeployment(policy types.PolicyType, repo *types.Repository, deployment v1beta1.Deployment) (updated v1beta1.Deployment, shouldUpdateDeployment bool, err error) {
func (p *Provider) checkUnversionedDeployment(policy types.PolicyType, repo *types.Repository, deployment v1beta1.Deployment) (updatePlan *UpdatePlan, shouldUpdateDeployment bool, err error) {
	updatePlan = &UpdatePlan{}

	eventRepoRef, err := image.Parse(repo.Name)
	if err != nil {
		return
	}

	labels := deployment.GetLabels()
	log.WithFields(log.Fields{
		"labels":    labels,
		"name":      deployment.Name,
		"namespace": deployment.Namespace,
		"policy":    policy,
	}).Info("provider.kubernetes.checkVersionedDeployment: keel policy found, checking deployment...")

	shouldUpdateDeployment = false

	for idx, c := range deployment.Spec.Template.Spec.Containers {
		// Remove version if any
		// containerImageName := versionreg.ReplaceAllString(c.Image, "")

		containerImageRef, err := image.Parse(c.Image)
		if err != nil {
			log.WithFields(log.Fields{
				"error":      err,
				"image_name": c.Image,
			}).Error("provider.kubernetes: failed to parse image name")
			continue
		}

		log.WithFields(log.Fields{
			"name":              deployment.Name,
			"namespace":         deployment.Namespace,
			"parsed_image_name": containerImageRef.Remote(),
			"target_image_name": repo.Name,
			"target_tag":        repo.Tag,
			"policy":            policy,
			"image":             c.Image,
		}).Info("provider.kubernetes: checking image")

		if containerImageRef.Repository() != eventRepoRef.Repository() {
			log.WithFields(log.Fields{
				"parsed_image_name": containerImageRef.Remote(),
				"target_image_name": repo.Name,
			}).Info("provider.kubernetes: images do not match, ignoring")
			continue
		}

		// updating image
		if containerImageRef.Registry() == image.DefaultRegistryHostname {
			c.Image = fmt.Sprintf("%s:%s", containerImageRef.ShortName(), repo.Tag)
		} else {
			c.Image = fmt.Sprintf("%s:%s", containerImageRef.Repository(), repo.Tag)
		}

		deployment.Spec.Template.Spec.Containers[idx] = c
		// marking this deployment for update
		shouldUpdateDeployment = true

		// Update digest on env var
		deployment.Spec.Template.Spec.Containers[idx].Env = updateEnv(c.Env)

		// updating annotations
		annotations := deployment.GetAnnotations()
		// updating digest if available
		if repo.Digest != "" {

			// annotations[types.KeelDigestAnnotation+"/"+containerImageRef.Remote()] = repo.Digest
		}

		// adding image for updates
		annotations = addImageToPull(annotations, c.Image)

		deployment.SetAnnotations(annotations)

		updatePlan.CurrentVersion = containerImageRef.Tag()
		updatePlan.NewVersion = repo.Tag
		updatePlan.Deployment = deployment

		log.WithFields(log.Fields{
			"parsed_image":     containerImageRef.Remote(),
			"raw_image_name":   c.Image,
			"target_image":     repo.Name,
			"target_image_tag": repo.Tag,
			"policy":           policy,
		}).Info("provider.kubernetes: impacted deployment container found")

	}

	return updatePlan, shouldUpdateDeployment, nil
}

func updateEnv(env []apiv1.EnvVar) []apiv1.EnvVar {
	newEnv := make([]apiv1.EnvVar, 0)
	for _, e := range env {
		if e.Name != "KEELIO_DEPLOYMENT" {
			newEnv = append(newEnv, e)
		}
	}
	newEnv = append(newEnv, apiv1.EnvVar{Name: "KEELIO_DEPLOYMENT",
		Value: fmt.Sprintf("%x", time.Now().Unix())})
	return newEnv
}
