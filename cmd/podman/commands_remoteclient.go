// +build remoteclient

package main

import (
	"github.com/spf13/cobra"
)

const remoteclient = true

// commands that only the remoteclient implements
func getMainCommands() []*cobra.Command {
	return []*cobra.Command{}
}

// commands that only the remoteclient implements
func getAppCommands() []*cobra.Command {
	return []*cobra.Command{}
}

// commands that only the remoteclient implements
func getImageSubCommands() []*cobra.Command {
	return []*cobra.Command{}
}

// commands that only the remoteclient implements
func getContainerSubCommands() []*cobra.Command {
	return []*cobra.Command{}
}

// commands that only the remoteclient implements
func getGenerateSubCommands() []*cobra.Command {
	return []*cobra.Command{}
}

// commands that only the remoteclient implements
func getPlaySubCommands() []*cobra.Command {
	return []*cobra.Command{}
}

// commands that only the remoteclient implements
func getTrustSubCommands() []*cobra.Command {
	return []*cobra.Command{}
}

// commands that only the remoteclient implements
func getSystemSubCommands() []*cobra.Command {
	return []*cobra.Command{}
}

// Commands that the remoteclient implements
func getHealthcheckSubCommands() []*cobra.Command {
	return []*cobra.Command{}
}
