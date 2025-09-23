/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"mona-actions/gh-migration-validator/internal/api"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var ghAPI *api.GitHubAPI

func initializeAPI() {
	ghAPI = api.GetAPI()
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "gh-migration-validator",
	Short: "Validate GitHub organization migrations",
	Long: `A GitHub CLI extension for validating GitHub organization migrations.

This tool helps ensure that your migration from one GitHub organization to another
has been completed successfully by comparing certain repositories resources
between source and target organizations.`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	Run: func(cmd *cobra.Command, args []string) {

		// Get parameters
		sourceOrganization := cmd.Flag("source-organization").Value.String()
		targetOrganization := cmd.Flag("target-organization").Value.String()
		sourceToken := cmd.Flag("source-token").Value.String()
		targetToken := cmd.Flag("target-token").Value.String()
		ghHostname := cmd.Flag("source-hostname").Value.String()

		// Set ENV variables
		os.Setenv("GHMV_SOURCE_ORGANIZATION", sourceOrganization)
		os.Setenv("GHMV_TARGET_ORGANIZATION", targetOrganization)
		os.Setenv("GHMV_SOURCE_TOKEN", sourceToken)
		os.Setenv("GHMV_TARGET_TOKEN", targetToken)
		os.Setenv("GHMV_SOURCE_HOSTNAME", ghHostname)

		// Bind ENV variables in Viper
		viper.BindEnv("SOURCE_ORGANIZATION")
		viper.BindEnv("TARGET_ORGANIZATION")
		viper.BindEnv("SOURCE_TOKEN")
		viper.BindEnv("TARGET_TOKEN")
		viper.BindEnv("SOURCE_HOSTNAME")
		viper.BindEnv("SOURCE_PRIVATE_KEY")
		viper.BindEnv("SOURCE_APP_ID")
		viper.BindEnv("SOURCE_INSTALLATION_ID")
		viper.BindEnv("TARGET_PRIVATE_KEY")
		viper.BindEnv("TARGET_APP_ID")
		viper.BindEnv("TARGET_INSTALLATION_ID")

		initializeAPI()

		// Below are just examples of how to use the API
		// You can create other functions that encapsulate the API calls to make the code cleaner
		// and easier to read.
		// You can also create a new file for each function and import it here
		// to keep the code organized.

		fmt.Println("Retrieving source user with REST API")
		sourceUser, err := ghAPI.GetSourceAuthenticatedUser()
		if err != nil {
			fmt.Println("Error retrieving source user with REST API")
			fmt.Println(err)
		}
		fmt.Println("Source user: ", sourceUser.GetLogin())

		fmt.Println("Retrieiving target user with REST API")

		targetuser, err := ghAPI.GetTargetAuthenticatedUser()
		if err != nil {
			fmt.Println("Error retrieving target user with REST API")
			fmt.Println(err)
		}
		fmt.Println("Target user: ", targetuser.GetLogin())

		fmt.Println("Retrieiving source user with GraphQL API")
		sourceGQLuser, err := ghAPI.GetSourceGraphQLAuthenticatedUser()
		if err != nil {
			fmt.Println("Error retrieving source user with GraphQL API")
			fmt.Println(err)
		}
		fmt.Println("Source graphql user: ", sourceGQLuser.GetLogin())

		fmt.Println("Retrieiving target user with GraphQL API")
		targetGQLUser, err := ghAPI.GetTargetGraphQLAuthenticatedUser()
		if err != nil {
			fmt.Println("Error retrieving target user with GraphQL API")
			fmt.Println(err)
		}
		fmt.Println("Target graphql user: ", targetGQLUser.GetLogin())
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.gh-migration-validator.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.

	rootCmd.Flags().StringP("source-organization", "s", "", "Source Organization to sync teams from")
	//rootCmd.MarkFlagRequired("source-organization")

	rootCmd.Flags().StringP("target-organization", "t", "", "Target Organization to sync teams from")
	//rootCmd.MarkFlagRequired("target-organization")

	rootCmd.Flags().StringP("source-token", "a", "", "Source Organization GitHub token. Scopes: read:org, read:user, user:email")
	rootCmd.MarkFlagRequired("source-token")

	rootCmd.Flags().StringP("target-token", "b", "", "Target Organization GitHub token. Scopes: admin:org")
	rootCmd.MarkFlagRequired("target-token")

	rootCmd.Flags().StringP("source-hostname", "u", "", "GitHub Enterprise source hostname url (optional) Ex. https://github.example.com")

	viper.SetEnvPrefix("GHMV") // Set the environment variable prefix, GHMV (GitHub Migration Validator)

	// Read in environment variables that match
	viper.AutomaticEnv()
}
