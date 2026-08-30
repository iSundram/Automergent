package commands

// Default returns the default command registry with all commands registered.
// This is the single source of truth: palette, help and dispatch are all
// derived from it and must not be duplicated elsewhere.
//
// Each command lives in its own cmd_<name>.go file (definition + handler);
// this file is only the ordered registration list.

func Default() *Registry {
	r := NewRegistry()

	// --- AI & Model ---
	r.MustRegister(modelCommand(), handleModel)
	r.MustRegister(providerCommand(), handleProvider)
	r.MustRegister(modeCommand(), handleMode)
	r.MustRegister(contextCommand(), handleContext)
	r.MustRegister(costCommand(), handleCost)
	r.MustRegister(compactCommand(), handleCompact)

	// --- Session ---
	r.MustRegister(newCommand(), handleNew)
	r.MustRegister(sessionsCommand(), handleSessions)
	r.MustRegister(resumeCommand(), handleResume)
	r.MustRegister(exportCommand(), handleExport)
	r.MustRegister(permissionsCommand(), handlePermissions)
	r.MustRegister(rewindCommand(), handleRewind)
	r.MustRegister(branchCommand(), handleBranch)
	r.MustRegister(summaryCommand(), handleSummary)
	r.MustRegister(renameCommand(), handleRename)
	r.MustRegister(recapCommand(), handleRecap)
	r.MustRegister(goalCommand(), handleGoal)
	r.MustRegister(copyCommand(), handleCopy)

	// --- Project ---
	r.MustRegister(treeCommand(), handleTree)
	r.MustRegister(diffCommand(), handleDiff)
	r.MustRegister(searchCommand(), handleSearch)
	r.MustRegister(initCommand(), handleInit)
	r.MustRegister(filesCommand(), handleFiles)
	r.MustRegister(addDirCommand(), handleAddDir)
	r.MustRegister(directoryCommand(), handleDirectory)
	r.MustRegister(lspCommand(), handleLsp)

	// --- Workflow ---
	r.MustRegister(runCommand(), handleRun)
	r.MustRegister(testCommand(), handleTest)
	r.MustRegister(buildCommand(), handleBuild)
	r.MustRegister(commitCommand(), handleCommit)
	r.MustRegister(reviewCommand(), handleReview)
	r.MustRegister(securityReviewCommand(), handleSecurityReview)
	r.MustRegister(issueCommand(), handleIssue)
	r.MustRegister(prCommentsCommand(), handlePRComments)
	r.MustRegister(reviewModeCommand(), handleReviewMode)
	r.MustRegister(cancelCommand(), handleCancel)
	r.MustRegister(planCommand(), handlePlan)
	r.MustRegister(artifactCommand(), handleArtifact)

	// --- Configuration ---
	r.MustRegister(apiKeyCommand(), handleAPIKey)
	r.MustRegister(baseURLCommand(), handleBaseURL)
	r.MustRegister(effortCommand(), handleEffort)
	r.MustRegister(providerAPIKeyCommand(), handleProviderAPIKey)
	r.MustRegister(providerBaseURLCommand(), handleProviderBaseURL)
	r.MustRegister(themeCommand(), handleTheme)
	r.MustRegister(keybindingsCommand(), handleKeybindings)
	r.MustRegister(memoryCommand(), handleMemory)
	r.MustRegister(configCommand(), handleConfig)

	// --- System ---
	r.MustRegister(envCommand(), handleEnv)
	r.MustRegister(versionCommand(), handleVersion)
	r.MustRegister(doctorCommand(), handleDoctor)
	r.MustRegister(statsCommand(), handleStats)
	r.MustRegister(errorsCommand(), handleErrors)
	r.MustRegister(helpCommand(), handleHelp)
	r.MustRegister(quitCommand(), handleQuit)
	r.MustRegister(feedbackCommand(), handleFeedback)
	r.MustRegister(commandsCommand(r), commandsHandler(r))

	// --- MCP ---
	r.MustRegister(mcpCommand(), handleMCP)

	// --- Knowledge ---
	r.MustRegister(skillsCommand(r), handleSkills(r))
	r.MustRegister(agentsCommand(), handleAgents)
	r.MustRegister(tldrCommand(), handleTldr)

	return r
}
