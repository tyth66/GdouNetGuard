package campus

// GuardArgs returns the CLI arguments needed to re-launch the guard process
// with the same configuration. The caller is expected to have already called
// SaveConfigFile so that the target process can read settings from the config
// file via -config.
func GuardArgs(cfg Config, includeBackground bool) []string {
	args := make([]string, 0, 16)
	if includeBackground {
		args = append(args, "-background")
	}
	args = append(args, "-config", cfg.ConfigFile)

	// Pass credential-file if it differs from the default (empty).
	if cfg.CredentialFile != "" {
		args = append(args, "-credential-file", cfg.CredentialFile)
	}
	return args
}
