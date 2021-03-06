package octopusdeploy

import (
	"encoding/json"
	"log"
	"strconv"
	"strings"

	"github.com/mshetland/go-octopusdeploy/octopusdeploy"
	"github.com/hashicorp/terraform/helper/schema"
)

func resourceDeploymentStepIisWebsite() *schema.Resource {
	schemaRes := &schema.Resource{
		Create: resourceDeploymentStepIisWebsiteCreate,
		Read:   resourceDeploymentStepIisWebsiteRead,
		Update: resourceDeploymentStepIisWebsiteUpdate,
		Delete: resourceDeploymentStepIisWebsiteDelete,

		Schema: map[string]*schema.Schema{
			"website_name": {
				Type:        schema.TypeString,
				Description: "The name of the Website to be created",
				Required:    true,
			},
			"deployment_type": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"path_type": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"relative_path": {
				Type:        schema.TypeString,
				Description: "Relative Path to package Root for the physical Path",
				Optional:    true,
			},
			"start_web_site": {
				Type:        schema.TypeBool,
				Description: "Start Web Site",
				Optional:    true,
				Default:     true,
			},
			"anonymous_authentication": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Whether IIS should allow anonymous authentication.",
				Default:     false,
			},
			"basic_authentication": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Whether IIS should allow basic authentication with a 401 challenge.",
				Default:     false,
			},
			"windows_authentication": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Whether IIS should allow integrated Windows authentication with a 401 challenge.",
				Default:     true,
			},
			"binding": &schema.Schema{
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"protocol": {
							Type:        schema.TypeString,
							Description: "Protocol to bind to",
							Optional:    true,
							Default:     "https",
							ValidateFunc: validateValueFunc([]string{
								"http",
								"https",
							}),
						},
						"ip": {
							Type:        schema.TypeString,
							Description: "IP Address to bind to",
							Optional:    true,
							Default:     "*",
						},
						"port": {
							Type:        schema.TypeString,
							Description: "Port to bind to",
							Optional:    true,
							Default:     "*",
						},
						"host": {
							Type:        schema.TypeString,
							Description: "Host Name to bind to",
							Optional:    true,
							Default:     "",
						},
						"enable": {
							Type:        schema.TypeBool,
							Optional:    true,
							Description: "Enable the binding",
							Default:     true,
						},
						"thumbprint": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "Thumbprint for the SSL Binding",
							Default:     "",
						},
						"cert_var": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "Certicate Variable Name for the SSL Binding",
							Default:     "",
						},
						"require_sni": {
							Type:        schema.TypeBool,
							Optional:    true,
							Description: "Require Service Name Identification for the SSL binding",
							Default:     false,
						},
					},
				},
			},
		},
	}

	/* Add Shared Schema's */
	resourceDeploymentStep_AddDefaultSchema(schemaRes, true)
	resourceDeploymentStep_AddPackageSchema(schemaRes)
	resourceDeploymentStep_AddIisAppPoolSchema(schemaRes)

	/* Return Schema */
	return schemaRes
}

func buildIisWebsiteDeploymentStep(d *schema.ResourceData) *octopusdeploy.DeploymentStep {
	/* Set Computed Values */
	d.Set("deployment_type", "webSite")

	/* Create Basic Deployment Step */
	deploymentStep := resourceDeploymentStep_CreateBasicStep(d, "Octopus.IIS")

	/* Enable IIS Web Site Feature */
	deploymentStep.Actions[0].Properties["Octopus.Action.EnabledFeatures"] = "Octopus.Features.IISWebSite"

	/* Add Shared Properties */
	resourceDeploymentStep_AddPackageProperties(d, deploymentStep)
	resourceDeploymentStep_AddIisAppPoolProperties(d, deploymentStep, "IISWebSite")

	/* Add Web Site Properties */
	deploymentStep.Actions[0].Properties["Octopus.Action.IISWebSite.DeploymentType"] = d.Get("deployment_type").(string)
	deploymentStep.Actions[0].Properties["Octopus.Action.IISWebSite.CreateOrUpdateWebSite"] = "True"
	deploymentStep.Actions[0].Properties["Octopus.Action.IISWebSite.WebApplication.CreateOrUpdate"] = "False"
	deploymentStep.Actions[0].Properties["Octopus.Action.IISWebSite.VirtualDirectory.CreateOrUpdate"] = "False"

	if relativePath, ok := d.GetOk("relative_path"); ok {
		d.Set("path_type", "relativeToPackageRoot")
		deploymentStep.Actions[0].Properties["Octopus.Action.IISWebSite.PhysicalPath"] = relativePath.(string)
	} else {
		d.Set("path_type", "packageRoot")
	}
	deploymentStep.Actions[0].Properties["Octopus.Action.IISWebSite.WebRootType"] = d.Get("path_type").(string)

	deploymentStep.Actions[0].Properties["Octopus.Action.IISWebSite.StartWebSite"] = formatBool(d.Get("start_web_site").(bool))

	deploymentStep.Actions[0].Properties["Octopus.Action.IISWebSite.WebSiteName"] = d.Get("website_name").(string)
	deploymentStep.Actions[0].Properties["Octopus.Action.IISWebSite.EnableAnonymousAuthentication"] = formatBool(d.Get("anonymous_authentication").(bool))
	deploymentStep.Actions[0].Properties["Octopus.Action.IISWebSite.EnableBasicAuthentication"] = formatBool(d.Get("basic_authentication").(bool))
	deploymentStep.Actions[0].Properties["Octopus.Action.IISWebSite.EnableWindowsAuthentication"] = formatBool(d.Get("windows_authentication").(bool))

	/* Flatten Bindings */
	type bindingsStruct struct {
		Protocol            *string `json:"protocol"`
		IpAddress           *string `json:"ipAddress"`
		Port                *string `json:"port"`
		Host                *string `json:"host"`
		Thumbprint          *string `json:"thumbprint"`
		CertificateVariable *string `json:"certificateVariable"`
		RequireSni          bool    `json:"requireSni"`
		Enabled             bool    `json:"enabled"`
	}

	bindingsArray := []bindingsStruct{}

	if rawBindings, ok := d.GetOk("binding"); ok {
		bindings := rawBindings.([]interface{})

		for _, rawBinding := range bindings {
			binding := rawBinding.(map[string]interface{})

			bindingsArray = append(bindingsArray, bindingsStruct{
				formatStrPtr(binding["protocol"].(string)),
				formatStrPtr(binding["ip"].(string)),
				formatStrPtr(binding["port"].(string)),
				formatStrPtr(binding["host"].(string)),
				formatStrPtr(binding["thumbprint"].(string)),
				formatStrPtr(binding["cert_var"].(string)),
				binding["require_sni"].(bool),
				binding["enable"].(bool),
			})
		}
	} else {
		log.Printf("rawBindings: %+v", rawBindings)
		log.Printf("getBindingsOk: %t", ok)

		/* Add Default HTTP 80 binding */
		bindingsArray = append(bindingsArray, bindingsStruct{
			formatStrPtr("http"),
			formatStrPtr("*"),
			formatStrPtr("80"),
			formatStrPtr(""),
			formatStrPtr(""),
			formatStrPtr(""),
			false,
			true,
		})
	}

	log.Printf("bindingsArray: %+v", bindingsArray)

	bindingsBytes, _ := json.Marshal(bindingsArray)
	bindingsString := strings.ReplaceAll(string(bindingsBytes), "\"", "\\\"")

	log.Printf("bindingsString: %s", bindingsString)

	deploymentStep.Actions[0].Properties["Octopus.Action.IISWebSite.Bindings"] = string(bindingsBytes)

	/* Return Deployment Step */
	return deploymentStep
}

func setIisWebsiteSchema(d *schema.ResourceData, deploymentStep octopusdeploy.DeploymentStep) {
	resourceDeploymentStep_SetBasicSchema(d, deploymentStep)
	resourceDeploymentStep_SetPackageSchema(d, deploymentStep)
	resourceDeploymentStep_SetIisAppPoolSchema(d, deploymentStep, "IISWebSite")

	/* Get Web Site Properties */
	d.Set("deployment_type", deploymentStep.Actions[0].Properties["Octopus.Action.IISWebSite.DeploymentType"])

	if pathType, ok := deploymentStep.Actions[0].Properties["Octopus.Action.IISWebSite.WebRootType"]; ok {
		d.Set("path_type", pathType)
	}

	if relativePath, ok := deploymentStep.Actions[0].Properties["Octopus.Action.IISWebSite.PhysicalPath"]; ok {
		d.Set("relative_path", relativePath)
	}

	if startWebSiteString, ok := deploymentStep.Actions[0].Properties["Octopus.Action.IISWebSite.StartWebSite"]; ok {
		if startWebSite, err := strconv.ParseBool(startWebSiteString); err == nil {
			d.Set("start_web_site", startWebSite)
		}
	}

	if websiteName, ok := deploymentStep.Actions[0].Properties["Octopus.Action.IISWebSite.WebSiteName"]; ok {
		d.Set("website_name", websiteName)
	}

	if anonymousAuthenticationString, ok := deploymentStep.Actions[0].Properties["Octopus.Action.IISWebSite.EnableAnonymousAuthentication"]; ok {
		if anonymousAuthentication, err := strconv.ParseBool(anonymousAuthenticationString); err == nil {
			d.Set("anonymous_authentication", anonymousAuthentication)
		}
	}

	if basicAuthenticationString, ok := deploymentStep.Actions[0].Properties["Octopus.Action.IISWebSite.EnableBasicAuthentication"]; ok {
		if basicAuthentication, err := strconv.ParseBool(basicAuthenticationString); err == nil {
			d.Set("basic_authentication", basicAuthentication)
		}
	}

	if windowsAuthenticationString, ok := deploymentStep.Actions[0].Properties["Octopus.Action.IISWebSite.EnableWindowsAuthentication"]; ok {
		if windowsAuthentication, err := strconv.ParseBool(windowsAuthenticationString); err == nil {
			d.Set("windows_authentication", windowsAuthentication)
		}
	}

	/* TODO: Expand Bindings */
	// deploymentStep.Actions[0].Properties["Octopus.Action.IISWebSite.Bindings"]
}

func resourceDeploymentStepIisWebsiteCreate(d *schema.ResourceData, m interface{}) error {
	return resourceDeploymentStepCreate(d, m, buildIisWebsiteDeploymentStep)
}

func resourceDeploymentStepIisWebsiteRead(d *schema.ResourceData, m interface{}) error {
	return resourceDeploymentStepRead(d, m, setIisWebsiteSchema)
}

func resourceDeploymentStepIisWebsiteUpdate(d *schema.ResourceData, m interface{}) error {
	return resourceDeploymentStepUpdate(d, m, buildIisWebsiteDeploymentStep)
}

func resourceDeploymentStepIisWebsiteDelete(d *schema.ResourceData, m interface{}) error {
	return resourceDeploymentStepDelete(d, m)
}
