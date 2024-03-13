package resource

import (
	"fmt"

	"github.com/containers/podman/v5/pkg/k8s.io/api/core/v1"
	"github.com/containers/podman/v5/pkg/k8s.io/api/resource/v1alpha2"
	metav1 "github.com/containers/podman/v5/pkg/k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	PodmanResourceClassName = "PodmanResourceClass"
	ClaimParametersKind     = "ClaimParameters"
	// because ClaimParameters are usually CRD's we get to define the ApiVersion values
	CDIClaimParametersApiVersion          = "cdi.resource.podman.io/v1"
	CDIResourceClassName                  = "cdidevice.podman.io"
	SimpleDeviceClaimParametersApiVersion = "simpledevice.resource.podman.io/v1"
	SimpleDeviceResourceClassName         = "simpledevice.podman.io"
)

type ClaimParameters struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// ClaimParameters spec. In a normal K8s cluster the ClaimParameters api object would most
	// likely be a CRD (custom resource definition) that ResourceClass would know how to handle
	// because Podman is *not* K8s we are going to define a support two. One for CDI devices and
	// the other for "simple" devices. Because we are defining these and there are only two we
	// will use just one "ClaimParameters" struct with a map for the parameters since the two we
	// are supporting only need that
	Spec map[string]string `json:"spec"`
}

type DynamicResourcesManager struct {
	resourceClaimParameters map[string]ClaimParameters
	// TODO figure out
	resourceClaimTemplates map[string]v1alpha2.ResourceClaim
}

func NewDynamicDevicesResourceManager() DynamicResourcesManager {
	dm := DynamicResourcesManager{
		resourceClaimParameters: make(map[string]ClaimParameters),
		resourceClaimTemplates:  make(map[string]v1alpha2.ResourceClaim),
	}
	return dm
}

func (dm DynamicResourcesManager) AddClaimParameters(resourceClaimParameters ClaimParameters) error {
	if _, ok := dm.resourceClaimParameters[resourceClaimParameters.Name]; ok {
		return fmt.Errorf("duplicate resource claim parameters defined")
	}
	dm.resourceClaimParameters[resourceClaimParameters.Name] = resourceClaimParameters
	return nil
}

func (dm DynamicResourcesManager) AddResourceClaimTemplate(resourceClaimTemplate v1alpha2.ResourceClaim) error {
	if _, ok := dm.resourceClaimTemplates[resourceClaimTemplate.Name]; ok {
		return fmt.Errorf("duplicate resource claim template defined")
	}

	dm.resourceClaimTemplates[resourceClaimTemplate.Name] = resourceClaimTemplate
	return nil
}

// TODO remove when feature is figured out
func (dm DynamicResourcesManager) PrintState() {
	fmt.Println("Templates")
	for _, t := range dm.resourceClaimTemplates {
		fmt.Printf("  %s - %s\n", t.APIVersion, t.Name)
	}

	fmt.Println("Claim parameters")
	for _, t := range dm.resourceClaimParameters {
		fmt.Printf("  %s - %s\n", t.APIVersion, t.Name)
	}

}

// ResolveK8sResourceClaimToDevice takes a name within a container's resources.claims[*].name in the PodSpec
// and resolves it to either a simple Linux device or a CDI device name to be added to the SpecGen during
// container creation
func (dm DynamicResourcesManager) ResolveK8sPodResourceClaimToDevice(claim *v1.PodResourceClaim) (string, error) {
	errorMsgTmpl := "failed to resolve resource claim to device: %s"

	if claim.Source.ResourceClaimName != nil {
		return "", fmt.Errorf(errorMsgTmpl, "resource claim should be nil")
	}

	resourceClaimTemplate, err := dm.resolveK8sPodResourceClaimToResourceClaimTemplate(claim)
	if err != nil {
		return "", fmt.Errorf(errorMsgTmpl, err)
	}

	return dm.resolveResourceClaimTemplateToDevice(resourceClaimTemplate)
}

// resolveK8sResourceClaimToDevice takes a PodResourceClaim and resolves the source
// to either a ResourceClaimName or a ResourceClaimTemplateName
func (dm DynamicResourcesManager) resolveK8sPodResourceClaimToResourceClaimTemplate(claim *v1.PodResourceClaim) (v1alpha2.ResourceClaim, error) {

	if claim.Source.ResourceClaimTemplateName == nil {
		return v1alpha2.ResourceClaim{}, fmt.Errorf("claim source missing template name")
	}

	resourceClaimTemplate, ok := dm.resourceClaimTemplates[*claim.Source.ResourceClaimTemplateName]
	if !ok {
		return v1alpha2.ResourceClaim{}, fmt.Errorf("Pod Resource Claim Source not found")
	}

	return resourceClaimTemplate, nil
}

// resolveResourceClaimSource takes a ClaimSource and returns a device that can be injected into the SpecGen for the pod
// Podman only supports "simple" devices (/dev/something) and CDI devices (vendor.com/device=name)
func (dm DynamicResourcesManager) resolveResourceClaimTemplateToDevice(rt v1alpha2.ResourceClaim) (string, error) {
	parameters, ok := dm.resourceClaimParameters[rt.Spec.ParametersRef.Name]
	if !ok {
		return "", fmt.Errorf("failed to resolve resource claim parameters")
	}

	if parameters.APIVersion == SimpleDeviceClaimParametersApiVersion {
		var device string
		if device, ok = parameters.Spec["hostpath"]; !ok {
			return "", fmt.Errorf("missing hostpath in simple device resource claim parameters")
		}
		return device, nil
	} else if parameters.APIVersion == CDIClaimParametersApiVersion {
		var ok bool
		var devicePart, vendorPart, namePart string
		errTmpl := "missing %s parameter of CDI claim parameter resource"
		if devicePart, ok = parameters.Spec["device"]; !ok {
			return "", fmt.Errorf(errTmpl, "device")
		}
		if vendorPart, ok = parameters.Spec["vendor"]; !ok {
			return "", fmt.Errorf(errTmpl, "vendor")
		}
		if namePart, ok = parameters.Spec["name"]; !ok {
			return "", fmt.Errorf(errTmpl, "name")
		}

		return fmt.Sprintf("%s/%s=%s", vendorPart, devicePart, namePart), nil
	} else {
		return "", fmt.Errorf("unsupported resource claim parameter apiVersion: %s", parameters.APIVersion)
	}
}
