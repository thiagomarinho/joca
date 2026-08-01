package addwizard

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type awsProfileSuggestion struct {
	Name   string
	Role   string
	Region string
}

type awsCredentialSuggestion struct {
	value   string
	detail  string
	profile *awsProfileSuggestion
}

var commonAWSRegions = []string{
	"af-south-1",
	"ap-east-1",
	"ap-northeast-1",
	"ap-northeast-2",
	"ap-northeast-3",
	"ap-south-1",
	"ap-south-2",
	"ap-southeast-1",
	"ap-southeast-2",
	"ap-southeast-3",
	"ap-southeast-4",
	"ca-central-1",
	"ca-west-1",
	"eu-central-1",
	"eu-central-2",
	"eu-north-1",
	"eu-south-1",
	"eu-south-2",
	"eu-west-1",
	"eu-west-2",
	"eu-west-3",
	"il-central-1",
	"me-central-1",
	"me-south-1",
	"sa-east-1",
	"us-east-1",
	"us-east-2",
	"us-west-1",
	"us-west-2",
}

func discoverAWSProfiles() []awsProfileSuggestion {
	home, _ := os.UserHomeDir()
	configFile := os.Getenv("AWS_CONFIG_FILE")
	if configFile == "" {
		configFile = filepath.Join(home, ".aws", "config")
	}
	credentialsFile := os.Getenv("AWS_SHARED_CREDENTIALS_FILE")
	if credentialsFile == "" {
		credentialsFile = filepath.Join(home, ".aws", "credentials")
	}
	return loadAWSProfileSuggestions(configFile, credentialsFile)
}

func loadAWSProfileSuggestions(paths ...string) []awsProfileSuggestion {
	profiles := make(map[string]awsProfileSuggestion)
	for _, path := range paths {
		loadAWSProfileFile(path, profiles)
	}
	result := make([]awsProfileSuggestion, 0, len(profiles))
	for _, profile := range profiles {
		result = append(result, profile)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name == "default" {
			return true
		}
		if result[j].Name == "default" {
			return false
		}
		return result[i].Name < result[j].Name
	})
	return result
}

func loadAWSProfileFile(path string, profiles map[string]awsProfileSuggestion) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()

	var current string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			current = normalizeAWSProfileSection(section)
			if current != "" {
				profile := profiles[current]
				profile.Name = current
				profiles[current] = profile
			}
			continue
		}
		if current == "" || line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		profile := profiles[current]
		switch strings.TrimSpace(strings.ToLower(key)) {
		case "region":
			profile.Region = strings.TrimSpace(value)
		case "sso_role_name":
			profile.Role = strings.TrimSpace(value)
		case "role_arn":
			if profile.Role == "" {
				profile.Role = strings.TrimSpace(value)
			}
		}
		profiles[current] = profile
	}
}

func normalizeAWSProfileSection(section string) string {
	if section == "default" {
		return section
	}
	if strings.HasPrefix(section, "profile ") {
		return strings.TrimSpace(strings.TrimPrefix(section, "profile "))
	}
	if strings.HasPrefix(section, "sso-session ") || strings.HasPrefix(section, "services ") {
		return ""
	}
	return section
}

func (m Model) awsCredentialSuggestions() []awsCredentialSuggestion {
	if m.awsCredField == 0 {
		prefix := strings.ToLower(strings.TrimSpace(m.awsProfile))
		var suggestions []awsCredentialSuggestion
		for i := range m.awsProfiles {
			profile := &m.awsProfiles[i]
			if prefix != "" && !strings.HasPrefix(strings.ToLower(profile.Name), prefix) {
				continue
			}
			var details []string
			if profile.Role != "" {
				details = append(details, "role: "+profile.Role)
			}
			if profile.Region != "" {
				details = append(details, "region: "+profile.Region)
			}
			suggestions = append(suggestions, awsCredentialSuggestion{
				value:   profile.Name,
				detail:  strings.Join(details, "  "),
				profile: profile,
			})
		}
		return suggestions
	}

	regions := make(map[string]bool, len(commonAWSRegions)+len(m.awsProfiles))
	for _, region := range commonAWSRegions {
		regions[region] = true
	}
	for _, profile := range m.awsProfiles {
		if profile.Region != "" {
			regions[profile.Region] = true
		}
	}
	prefix := strings.ToLower(strings.TrimSpace(m.awsRegion))
	values := make([]string, 0, len(regions))
	for region := range regions {
		if prefix == "" || strings.HasPrefix(strings.ToLower(region), prefix) {
			values = append(values, region)
		}
	}
	sort.Strings(values)
	suggestions := make([]awsCredentialSuggestion, len(values))
	for i, region := range values {
		suggestions[i] = awsCredentialSuggestion{value: region}
	}
	return suggestions
}

func (m Model) applyAWSSuggestion() Model {
	suggestions := m.awsCredentialSuggestions()
	if len(suggestions) == 0 {
		return m
	}
	if m.awsSuggestion >= len(suggestions) {
		m.awsSuggestion = len(suggestions) - 1
	}
	suggestion := suggestions[m.awsSuggestion]
	if m.awsCredField == 0 {
		m.awsProfile = suggestion.value
		if suggestion.profile != nil && suggestion.profile.Region != "" {
			m.awsRegion = suggestion.profile.Region
		}
	} else {
		m.awsRegion = suggestion.value
	}
	m.awsSuggestion = 0
	return m
}

func (m Model) hasUnappliedAWSSuggestion() bool {
	suggestions := m.awsCredentialSuggestions()
	if len(suggestions) == 0 {
		return false
	}
	index := m.awsSuggestion
	if index >= len(suggestions) {
		index = len(suggestions) - 1
	}
	if m.awsCredField == 0 {
		return m.awsProfile != suggestions[index].value
	}
	return m.awsRegion != suggestions[index].value
}
