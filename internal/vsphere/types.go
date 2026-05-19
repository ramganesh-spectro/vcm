package vsphere

type VM struct {
	Name       string `json:"name"`
	Path       string `json:"path,omitempty"`
	PowerState string `json:"powerState"`
	IPAddress  string `json:"ipAddress,omitempty"`
	GuestOS    string `json:"guestOS,omitempty"`
	Host       string `json:"host,omitempty"`
	Datastore  string `json:"datastore,omitempty"`
	MoID       string `json:"moid,omitempty"`
}

type CloneSpec struct {
	Source    string `json:"source"`
	Name      string `json:"name"`
	Folder    string `json:"folder"`
	Datastore string `json:"datastore"`
	Pool      string `json:"pool,omitempty"`
	PowerOn   bool   `json:"powerOn"`
}

type CDAttachSpec struct {
	VM      string `json:"vm"`
	ISOPath string `json:"isoPath"`
	Device  int    `json:"device"`
}

type Datastore struct {
	Name          string `json:"name"`
	Type          string `json:"type,omitempty"`
	URL           string `json:"url,omitempty"`
	CapacityBytes int64  `json:"capacityBytes"`
	FreeBytes     int64  `json:"freeBytes"`
}
