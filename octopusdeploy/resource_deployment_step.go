package octopusdeploy

import (
	"fmt"
	"log"
	"strings"

	"github.com/OctopusDeploy/go-octopusdeploy/octopusdeploy"
	"github.com/hashicorp/terraform/helper/schema"
)

/* --------------------------------------- */
/* Shared Schema  Setups */
/* --------------------------------------- */
func resourceDeploymentStep_AddDefaultSchema(schemaRes *schema.Resource, target_roles_required bool) {

	schemaRes.Schema["project_id"] = &schema.Schema{
		Type:     schema.TypeString,
		Required: true,
		ForceNew: true,
	}

	schemaRes.Schema["deployment_process_id"] = &schema.Schema{
		Type:     schema.TypeString,
		Computed: true,
	}

	schemaRes.Schema["before_step_id"] = &schema.Schema{
		Type:        schema.TypeString,
		Description: "Define Step this should preceed, else will be added to the end at time of creation",
		Optional:    true,
	}

	schemaRes.Schema["after_step_id"] = &schema.Schema{
		Type:        schema.TypeString,
		Description: "Define Step this should follow, else will be added to the end at time of creation",
		Optional:    true,
	}

	schemaRes.Schema["step_name"] = &schema.Schema{
		Type:        schema.TypeString,
		Description: "The name of the deployment step.",
		Required:    true,
	}

	schemaRes.Schema["step_condition"] = &schema.Schema{
		Type:        schema.TypeString,
		Description: "Limit when this step will run by setting this condition.",
		Optional:    true,
		ValidateFunc: validateValueFunc([]string{
			"success",
			"failure",
			"always",
			"variable",
		}),
		Default: "success",
	}

	schemaRes.Schema["required"] = &schema.Schema{
		Type:     schema.TypeBool,
		Optional: true,
		Default:  false,
	}

	schemaRes.Schema["step_start_trigger"] = &schema.Schema{
		Type:        schema.TypeString,
		Optional:    true,
		Default:     "StartAfterPrevious",
		Description: "Control whether the step waits for the previous step to complete, or runs parallel with it.",
		ValidateFunc: validateValueFunc([]string{
			"StartAfterPrevious",
			"StartWithPrevious",
		}),
	}

	schemaRes.Schema["target_roles"] = &schema.Schema{
		Type:     schema.TypeList,
		Required: target_roles_required,
		Optional: !target_roles_required,
		Elem: &schema.Schema{
			Type: schema.TypeString,
		},
	}

	if !target_roles_required {
		schemaRes.Schema["run_on_server"] = &schema.Schema{
			Type:        schema.TypeBool,
			Description: "Whether the script runs on the server (true) or target (false)",
			Optional:    true,
			Default:     false,
		}
	}
}

func resourceDeploymentStep_AddPackageSchema(schemaRes *schema.Resource) {
	schemaRes.Schema["feed_id"] = &schema.Schema{
		Type:        schema.TypeString,
		Description: "The ID of the feed a package will be found in.",
		Required:    true,
	}

	schemaRes.Schema["package"] = &schema.Schema{
		Type:        schema.TypeString,
		Description: "ID / Name of the package to be deployed.",
		Required:    true,
	}

	schemaRes.Schema["configuration_transforms"] = &schema.Schema{
		Type:        schema.TypeBool,
		Description: "Enables XML configuration transformations.",
		Optional:    true,
		Default:     true,
	}

	schemaRes.Schema["configuration_variables"] = &schema.Schema{
		Type:        schema.TypeBool,
		Description: "Enables replacing appSettings and connectionString entries in any .config file.",
		Optional:    true,
		Default:     true,
	}

	schemaRes.Schema["json_file_variable_replacement"] = &schema.Schema{
		Type:        schema.TypeString,
		Optional:    true,
		Description: "A comma-separated list of file names to replace settings in, relative to the package contents.",
	}

	schemaRes.Schema["variable_substitution_in_files"] = &schema.Schema{
		Type:        schema.TypeString,
		Optional:    true,
		Description: "A newline-separated list of file names to transform, relative to the package contents. Extended wildcard syntax is supported.",
	}
}

/* --------------------------------------- */
/* Universal Create, Read, Update, Delete */
/* --------------------------------------- */
func resourceDeploymentStepCreate(d *schema.ResourceData, m interface{}, buildDeploymentProcessStepFunc func(d *schema.ResourceData) *octopusdeploy.DeploymentStep) error {
	client := m.(*octopusdeploy.Client)

	projectId := d.Get("project_id").(string)
	beforeStepId := d.Get("before_step_id").(string)
	afterStepId := d.Get("after_step_id").(string)

	/* Find Deployment Process */
	log.Printf("Loading Project Information '%s' ...", projectId)
	project, err := client.Project.Get(projectId)

	if err != nil {
		return fmt.Errorf("error loading project '%s': %s", projectId, err.Error())
	}

	log.Printf("Loading Deployment Process '%s' ...", project.DeploymentProcessID)
	deploymentProcess, err := client.DeploymentProcess.Get(project.DeploymentProcessID)

	if err != nil {
		return fmt.Errorf("error reading deployment process '%s': %s", project.DeploymentProcessID, err.Error())
	}

	/* Create Deployment Process Step */
	newDeploymentStep := buildDeploymentProcessStepFunc(d)

	/* Add Step Appropiately into Deployment Steps */
	orgDeploymentSteps := deploymentProcess.Steps

	deploymentProcess.Steps = nil // empty the steps
	newStepAddedIndex := -1
	for stepIndex, orgDeploymentStep := range orgDeploymentSteps {
		if newStepAddedIndex == -1 && orgDeploymentStep.ID == beforeStepId {
			newStepAddedIndex = stepIndex
			deploymentProcess.Steps = append(deploymentProcess.Steps, *newDeploymentStep)
		}

		deploymentProcess.Steps = append(deploymentProcess.Steps, orgDeploymentStep)

		if newStepAddedIndex == -1 && orgDeploymentStep.ID == afterStepId {
			newStepAddedIndex = stepIndex + 1
			deploymentProcess.Steps = append(deploymentProcess.Steps, *newDeploymentStep)
		}
	}

	if newStepAddedIndex == -1 {
		newStepAddedIndex = len(deploymentProcess.Steps)
		deploymentProcess.Steps = append(deploymentProcess.Steps, *newDeploymentStep)
	}

	// Update Deployment Process with new Step
	log.Printf("Updating Deployment Process '%s' ...", project.DeploymentProcessID)
	updateDeploymentProcess, err := client.DeploymentProcess.Update(deploymentProcess)

	if err != nil {
		return fmt.Errorf("error updating deployment process for project: %s", err.Error())
	}

	/* Set Ids */
	d.Set("deployment_process_id", updateDeploymentProcess.ID)
	d.SetId(updateDeploymentProcess.Steps[newStepAddedIndex].ID)

	/* Return */
	return nil
}

func resourceDeploymentStepRead(d *schema.ResourceData, m interface{}, setSchemaFunc func(d *schema.ResourceData, deploymentStep octopusdeploy.DeploymentStep)) error {
	client := m.(*octopusdeploy.Client)

	/* Get Id's */
	stepId := d.Id()
	processId := d.Get("deployment_process_id").(string)

	/* Load Step Information */
	log.Printf("Loading Deployment Process '%s' ...", processId)
	deploymentProcess, err := client.DeploymentProcess.Get(processId)

	if err == octopusdeploy.ErrItemNotFound {
		d.SetId("")
		return nil
	}

	if err != nil {
		return fmt.Errorf("error reading deployment process '%s': %s", processId, err.Error())
	}

	var deploymentStep *octopusdeploy.DeploymentStep
	for _, findDeploymentStep := range deploymentProcess.Steps {
		if findDeploymentStep.ID == stepId {
			deploymentStep = &findDeploymentStep
			break
		}
	}

	if deploymentStep == nil {
		d.SetId("")
		return nil
	}

	/* Set Schema */
	setSchemaFunc(d, *deploymentStep)

	return nil
}

func resourceDeploymentStepUpdate(d *schema.ResourceData, m interface{}, buildDeploymentProcessStepFunc func(d *schema.ResourceData) *octopusdeploy.DeploymentStep) error {
	client := m.(*octopusdeploy.Client)

	/* Get Id's */
	stepId := d.Id()
	processId := d.Get("deployment_process_id").(string)
	beforeStepId := d.Get("before_step_id").(string)
	afterStepId := d.Get("after_step_id").(string)

	/* Load Deployment Process */
	log.Printf("Loading Deployment Process '%s' ...", processId)
	deploymentProcess, err := client.DeploymentProcess.Get(processId)

	if err == octopusdeploy.ErrItemNotFound {
		d.SetId("")
		return nil
	}

	if err != nil {
		return fmt.Errorf("error reading deployment process id %s: %s", processId, err.Error())
	}

	/* Create Deployment Process Step */
	newDeploymentStep := buildDeploymentProcessStepFunc(d)
	newDeploymentStep.ID = stepId

	/* Update Step */
	orgDeploymentSteps := deploymentProcess.Steps
	deploymentProcess.Steps = nil // empty the steps

	newStepAddedIndex := -1
	for stepIndex, orgDeploymentStep := range orgDeploymentSteps {
		if newStepAddedIndex == -1 && orgDeploymentStep.ID == beforeStepId {
			newStepAddedIndex = stepIndex
			deploymentProcess.Steps = append(deploymentProcess.Steps, *newDeploymentStep)
		}

		if orgDeploymentStep.ID != stepId {
			deploymentProcess.Steps = append(deploymentProcess.Steps, orgDeploymentStep)
		}

		if newStepAddedIndex == -1 && orgDeploymentStep.ID == afterStepId {
			newStepAddedIndex = stepIndex + 1
			deploymentProcess.Steps = append(deploymentProcess.Steps, *newDeploymentStep)
		}
	}

	if newStepAddedIndex == -1 {
		newStepAddedIndex = len(deploymentProcess.Steps)
		deploymentProcess.Steps = append(deploymentProcess.Steps, *newDeploymentStep)
	}

	// Update Deployment Process with Step Removed
	log.Printf("Updating Deployment Process '%s' ...", processId)
	if _, err := client.DeploymentProcess.Update(deploymentProcess); err != nil {
		return fmt.Errorf("error updating deployment process for project: %s", err.Error())
	}

	return nil
}

func resourceDeploymentStepDelete(d *schema.ResourceData, m interface{}) error {
	client := m.(*octopusdeploy.Client)

	/* Get Id's */
	stepId := d.Id()
	processId := d.Get("deployment_process_id").(string)

	/* Load Deployment Process */
	log.Printf("Loading Deployment Process '%s' ...", processId)
	deploymentProcess, err := client.DeploymentProcess.Get(processId)

	if err == octopusdeploy.ErrItemNotFound {
		d.SetId("")
		return nil
	}

	if err != nil {
		return fmt.Errorf("error reading deployment process id %s: %s", processId, err.Error())
	}

	/* Remove Step */
	orgDeploymentSteps := deploymentProcess.Steps
	deploymentProcess.Steps = nil // empty the steps

	for _, orgDeploymentStep := range orgDeploymentSteps {
		if orgDeploymentStep.ID != stepId {
			deploymentProcess.Steps = append(deploymentProcess.Steps, orgDeploymentStep)
		}
	}

	// Update Deployment Process with Step Removed
	log.Printf("Updating Deployment Process '%s' ...", processId)
	if _, err := client.DeploymentProcess.Update(deploymentProcess); err != nil {
		return fmt.Errorf("error updating deployment process for project: %s", err.Error())
	}

	/* Set Id */
	d.SetId("")

	return nil
}

/* --------------------------------------- */
/* Shared Create Step Functions */
/* --------------------------------------- */
func resourceDeploymentStep_CreateBasicStep(d *schema.ResourceData, actionType string) *octopusdeploy.DeploymentStep {
	/* Get Basic Step Properties */
	stepName := d.Get("step_name").(string)
	stepCondition := d.Get("step_condition").(string)
	required := d.Get("required").(bool)
	stepStartTrigger := d.Get("step_start_trigger").(string)

	/* Create Deployment Step */
	deploymentStep := &octopusdeploy.DeploymentStep{
		Name:               stepName,
		PackageRequirement: "LetOctopusDecide",
		Condition:          octopusdeploy.DeploymentStepCondition(stepCondition),
		StartTrigger:       octopusdeploy.DeploymentStepStartTrigger(stepStartTrigger),
		Properties:         map[string]string{},
		Actions: []octopusdeploy.DeploymentAction{
			{
				Name:       stepName,
				IsRequired: required,
				ActionType: actionType,
				Properties: map[string]string{},
			},
		},
	}

	/* Add Run On Server */
	if runOnServer, ok := d.GetOk("run_on_server"); ok {
		deploymentStep.Properties["Octopus.Action.RunOnServer"] = formatBool(runOnServer.(bool))
	}

	/* Add Target Roles */
	if targetRolesInterface, ok := d.GetOk("target_roles"); ok {
		var targetRoleSlice []string

		targetRolesArray := targetRolesInterface.([]interface{})

		for _, role := range targetRolesArray {
			targetRoleSlice = append(targetRoleSlice, role.(string))
		}

		deploymentStep.Properties["Octopus.Action.TargetRoles"] = strings.Join(targetRoleSlice, ",")
	}

	/* Return */
	return deploymentStep
}

func resourceDeploymentStep_AddPackageProperties(d *schema.ResourceData, deploymentStep *octopusdeploy.DeploymentStep) {
	/* Package Properties */
	deploymentStep.Actions[0].Properties["Octopus.Action.Package.DownloadOnTentacle"] = "False";
	deploymentStep.Actions[0].Properties["Octopus.Action.Package.FeedId"] = d.Get("feed_id").(string)
	deploymentStep.Actions[0].Properties["Octopus.Action.Package.PackageId"] = d.Get("package").(string)

	/* Add Configuration Transformation Properties */
	if jsonFileVariableReplacement, ok := d.GetOk("json_file_variable_replacement"); ok {
		deploymentStep.Actions[0].Properties["Octopus.Action.Package.JsonConfigurationVariablesTargets"] = jsonFileVariableReplacement.(string)
		deploymentStep.Actions[0].Properties["Octopus.Action.Package.JsonConfigurationVariablesEnabled"] = "True"

		deploymentStep.Actions[0].Properties["Octopus.Action.EnabledFeatures"] += ",Octopus.Features.JsonConfigurationVariables"
	}

	if variableSubstitutionInFiles, ok := d.GetOk("variable_substitution_in_files"); ok {
		deploymentStep.Actions[0].Properties["Octopus.Action.SubstituteInFiles.TargetFiles"] = variableSubstitutionInFiles.(string)
		deploymentStep.Actions[0].Properties["Octopus.Action.SubstituteInFiles.Enabled"] = "True"

		deploymentStep.Actions[0].Properties["Octopus.Action.EnabledFeatures"] += ",Octopus.Features.SubstituteInFiles"
	}

	if configurationTransforms := d.Get("configuration_transforms").(bool); configurationTransforms {
		deploymentStep.Actions[0].Properties["Octopus.Action.Package.AutomaticallyRunConfigurationTransformationFiles"] = formatBool(configurationTransforms)
		deploymentStep.Actions[0].Properties["Octopus.Action.EnabledFeatures"] += ",Octopus.Features.ConfigurationTransforms"
	}

	if configurationVariables := d.Get("configuration_variables").(bool); configurationVariables {
		deploymentStep.Actions[0].Properties["Octopus.Action.Package.AutomaticallyUpdateAppSettingsAndConnectionStrings"] = formatBool(configurationVariables)
		deploymentStep.Actions[0].Properties["Octopus.Action.EnabledFeatures"] += ",Octopus.Features.ConfigurationVariables"
	}
}

/* --------------------------------------- */
/* Shared Set Schema Functions */
/* --------------------------------------- */
func resourceDeploymentStep_SetBasicSchema(d *schema.ResourceData, deploymentStep octopusdeploy.DeploymentStep) {
	d.Set("step_name", deploymentStep.Name)
}