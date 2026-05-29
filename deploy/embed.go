package deploy

import _ "embed"

//go:embed install-client.sh
var InstallClientScript []byte
