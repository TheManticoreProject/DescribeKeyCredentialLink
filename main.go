package main

import (
	"github.com/TheManticoreProject/Manticore/logger"
	"github.com/TheManticoreProject/Manticore/network/ldap"
	"github.com/TheManticoreProject/Manticore/windows/credentials"
	"github.com/TheManticoreProject/Manticore/windows/keycredentiallink"

	"fmt"

	"github.com/TheManticoreProject/goopts/parser"
)

var (
	// Configuration
	useLdaps bool
	debug    bool

	// Network settings
	domainController string
	ldapPort         int

	// Authentication details
	authDomain   string
	authUsername string
	authPassword string
	authHashes   string
	useKerberos  bool

	// Source values
	distinguishedName string
	rawValue          string
)

func parseArgs() {
	ap := parser.ArgumentsParser{Banner: "DescribeKeyCredentialLink - by Remi GASCOU (Podalirius) @ TheManticoreProject - v1.0.0"}

	// Configuration flags
	ap.NewBoolArgument(&debug, "", "--debug", false, "Debug mode.")

	// Source value
	group_sourceValues, err := ap.NewRequiredMutuallyExclusiveArgumentGroup("Source Values")
	if err != nil {
		fmt.Printf("[error] Error creating ArgumentGroup: %s\n", err)
	} else {
		group_sourceValues.NewStringArgument(&distinguishedName, "-D", "--distinguished-name", "", false, "Distinguished Name.")
		group_sourceValues.NewStringArgument(&rawValue, "-v", "--value", "", false, "Raw hex string value of of msDS-KeyCredentialLink, it typically starts with 'B:'.")
	}

	group_ldapSettings, err := ap.NewArgumentGroup("LDAP Connection Settings")
	if err != nil {
		fmt.Printf("[error] Error creating ArgumentGroup: %s\n", err)
	} else {
		group_ldapSettings.NewStringArgument(&domainController, "-dc", "--dc-ip", "", false, "IP Address of the domain controller or KDC (Key Distribution Center) for Kerberos. If omitted, it will use the domain part (FQDN) specified in the identity parameter.")
		group_ldapSettings.NewTcpPortArgument(&ldapPort, "-lp", "--ldap-port", 389, false, "Port number to connect to LDAP server.")
		group_ldapSettings.NewBoolArgument(&useLdaps, "-L", "--use-ldaps", false, "Use LDAPS instead of LDAP.")
		group_ldapSettings.NewBoolArgument(&useKerberos, "-k", "--use-kerberos", false, "Use Kerberos instead of NTLM.")
	}

	group_auth, err := ap.NewArgumentGroup("Authentication")
	if err != nil {
		fmt.Printf("[error] Error creating ArgumentGroup: %s\n", err)
	} else {
		group_auth.NewStringArgument(&authDomain, "-d", "--domain", "", false, "Active Directory domain to authenticate to.")
		group_auth.NewStringArgument(&authUsername, "-u", "--username", "", false, "User to authenticate as.")
		group_auth.NewStringArgument(&authPassword, "-p", "--password", "", false, "Password to authenticate with.")
		group_auth.NewStringArgument(&authHashes, "-H", "--hashes", "", false, "NT/LM hashes, format is LMhash:NThash.")
	}

	ap.Parse()

	if useLdaps && !group_ldapSettings.LongNameToArgument["--port"].IsPresent() {
		ldapPort = 636
	}
}

func describeSingleKeyCredential(rawDnWithBinaryValue []byte, debug bool) {
	kc := keycredentiallink.KeyCredentialLink{}

	if debug {
		logger.Debug(fmt.Sprintf("KeyCredential: %s", rawDnWithBinaryValue))
	}

	dnWithBinary := ldap.DNWithBinary{}
	_, err := dnWithBinary.Unmarshal(rawDnWithBinaryValue)
	if err != nil {
		logger.Warn(fmt.Sprintf("Error unmarshalling DNWithBinary: %s", err))
		return
	}

	err = kc.ParseDNWithBinary(dnWithBinary)
	if err != nil {
		logger.Warn(fmt.Sprintf("Error parsing DNWithBinary: %s", err))
		return
	}

	kc.Describe(0)
}

func main() {
	parseArgs()

	rawDnWithBinaryValue := [][]byte{}

	if len(rawValue) != 0 {
		rawDnWithBinaryValue = [][]byte{[]byte(rawValue)}
	} else if len(distinguishedName) != 0 {
		creds, err := credentials.NewCredentials(authDomain, authUsername, authPassword, authHashes)
		if err != nil {
			logger.Warn(fmt.Sprintf("Error creating credentials: %s", err))
			return
		}

		if debug {
			if !useLdaps {
				logger.Debug(fmt.Sprintf("Connecting to remote ldap://%s:%d ...", domainController, ldapPort))
			} else {
				logger.Debug(fmt.Sprintf("Connecting to remote ldaps://%s:%d ...", domainController, ldapPort))
			}
		}

		ldapSession, err := ldap.NewSession(
			domainController,
			ldapPort,
			creds,
			useLdaps,
			useKerberos,
		)
		if err != nil {
			logger.Warn(fmt.Sprintf("Error creating LDAP session: %s", err))
			return
		}

		connected, err := ldapSession.Connect()
		if err != nil {
			logger.Warn(fmt.Sprintf("Error connecting to LDAP: %s", err))
			return
		}

		if connected {
			logger.Info(fmt.Sprintf("Connected as '%s\\%s'", authDomain, authUsername))

			query := fmt.Sprintf("(distinguishedName=%s)", distinguishedName)

			if debug {
				logger.Debug(fmt.Sprintf("LDAP query used: %s", query))
			}

			attributes := []string{"distinguishedName", "msDS-KeyCredentialLink"}
			ldapResults, err := ldapSession.QueryWholeSubtree("", query, attributes)
			if err != nil {
				logger.Warn(fmt.Sprintf("Error querying LDAP: %s", err))
				return
			}

			for _, entry := range ldapResults {
				if debug {
					logger.Debug(fmt.Sprintf("| distinguishedName: %s", entry.GetAttributeValue("distinguishedName")))
				}
				rawDnWithBinaryValue = append(rawDnWithBinaryValue, entry.GetEqualFoldRawAttributeValues("msDS-KeyCredentialLink")...)
			}
		} else {
			if debug {
				logger.Warn("Error: Could not create ldapSession.")
			}
		}
	}

	if len(rawDnWithBinaryValue) != 0 {
		for i, value := range rawDnWithBinaryValue {
			logger.Info(fmt.Sprintf("Describing msDS-KeyCredentialLink entry %d:", i))
			describeSingleKeyCredential(value, debug)
		}
	} else {
		logger.Warn("No msDS-KeyCredentialLink found in source values.")
	}

	logger.Info("All done.")
}
