package network

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/hetznercloud/terraform-provider-hcloud/internal/util"
	"github.com/hetznercloud/terraform-provider-hcloud/internal/util/control"
	"github.com/hetznercloud/terraform-provider-hcloud/internal/util/hcloudutil"
)

// SubnetResourceType is the type name of the Hetzner Cloud Network Subnet resource.
const SubnetResourceType = "hcloud_network_subnet"

// SubnetResource creates a Terraform schema for the hcloud_network_subnet
// resource.
func SubnetResource() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceNetworkSubnetCreate,
		ReadContext:   resourceNetworkSubnetRead,
		DeleteContext: resourceNetworkSubnetDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"network_id": {
				Type:     schema.TypeInt,
				Required: true,
				ForceNew: true,
			},
			"type": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
				ValidateFunc: validation.StringInSlice([]string{
					"cloud",
					"server",
					"vswitch",
				}, false),
			},
			"network_zone": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"ip_range": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"gateway": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"vswitch_id": {
				Type:     schema.TypeInt,
				Optional: true,
				ForceNew: true,
			},
		},
	}
}

func resourceNetworkSubnetCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var a *hcloud.Action

	c := m.(*hcloud.Client)

	_, ipRange, err := net.ParseCIDR(d.Get("ip_range").(string))
	if err != nil {
		return hcloudutil.ErrorToDiag(err)
	}
	networkID := d.Get("network_id")
	network := &hcloud.Network{ID: util.CastInt64(networkID)}

	subnetType := hcloud.NetworkSubnetType(d.Get("type").(string))
	opts := hcloud.NetworkAddSubnetOpts{
		Subnet: hcloud.NetworkSubnet{
			IPRange:     ipRange,
			NetworkZone: hcloud.NetworkZone(d.Get("network_zone").(string)),
			Type:        subnetType,
		},
	}

	if subnetType == hcloud.NetworkSubnetTypeVSwitch {
		vSwitchID := d.Get("vswitch_id")
		opts.Subnet.VSwitchID = util.CastInt64(vSwitchID)
	}

	err = control.Retry(control.DefaultRetries, func() error {
		var err error

		a, _, err = c.Network.AddSubnet(ctx, network, opts)
		if hcloud.IsError(err, hcloud.ErrorCodeConflict) {
			return err
		}
		if hcloud.IsError(err, hcloud.ErrorCodeVSwitchAlreadyUsed) {
			return err
		}
		return control.AbortRetry(err)
	})
	if err != nil {
		return hcloudutil.ErrorToDiag(err)
	}
	d.SetId(generateNetworkSubnetID(network, ipRange.String()))

	if err := hcloudutil.WaitForAction(ctx, &c.Action, a); err != nil {
		return hcloudutil.ErrorToDiag(err)
	}

	return resourceNetworkSubnetRead(ctx, d, m)
}

func resourceNetworkSubnetRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*hcloud.Client)

	network, subnet, err := lookupNetworkSubnetID(ctx, d.Id(), client)
	if errors.Is(err, errInvalidNetworkSubnetID) {
		log.Printf("[WARN] Invalid id (%s), removing from state: %s", d.Id(), err)
		d.SetId("")
		return nil
	}
	if err != nil {
		return hcloudutil.ErrorToDiag(err)
	}
	if network == nil {
		log.Printf("[WARN] Network Subnet (%s) not found, removing from state", d.Id())
		d.SetId("")
		return nil
	}
	d.SetId(generateNetworkSubnetID(network, subnet.IPRange.String()))
	setNetworkSubnetSchema(d, network, subnet)
	return nil
}

func resourceNetworkSubnetDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var (
		a       *hcloud.Action
		network *hcloud.Network
	)

	c := m.(*hcloud.Client)

	err := control.Retry(control.DefaultRetries*10, func() error {
		var (
			subnet hcloud.NetworkSubnet
			err    error
		)

		network, subnet, err = lookupNetworkSubnetID(ctx, d.Id(), c)
		if err != nil {
			return control.AbortRetry(err)
		}

		a, _, err = c.Network.DeleteSubnet(ctx, network, hcloud.NetworkDeleteSubnetOpts{
			Subnet: subnet,
		})
		if hcloud.IsError(err, hcloud.ErrorCodeConflict) || hcloud.IsError(err, hcloud.ErrorCodeLocked) {
			return err
		}
		if hcloud.IsError(err, hcloud.ErrorCodeServiceError) &&
			(strings.Contains(err.Error(), "servers are attached") || strings.Contains(err.Error(), "network has attached resources")) {
			return err
		}
		return control.AbortRetry(err)
	})
	if hcloud.IsError(err, hcloud.ErrorCodeNotFound) || errors.Is(err, errInvalidNetworkSubnetID) {
		d.SetId("")
		return nil
	}
	if hcloud.IsError(err, hcloud.ErrorCodeServiceError) &&
		(strings.Contains(err.Error(), "servers are attached") || strings.Contains(err.Error(), "network has attached resources")) {
		log.Printf("[WARN] Network Subnet (%s) has still resources attached. We assume that the network will be deleted fully, removing from state", d.Id())
		d.SetId("")
		return nil
	}
	if err != nil {
		return hcloudutil.ErrorToDiag(err)
	}
	if err := hcloudutil.WaitForAction(ctx, &c.Action, a); err != nil {
		return hcloudutil.ErrorToDiag(err)
	}
	return nil
}

func setNetworkSubnetSchema(d *schema.ResourceData, n *hcloud.Network, s hcloud.NetworkSubnet) {
	d.SetId(generateNetworkSubnetID(n, s.IPRange.String()))
	d.Set("network_id", n.ID)
	d.Set("network_zone", s.NetworkZone)
	d.Set("ip_range", s.IPRange.String())
	d.Set("type", s.Type)
	d.Set("gateway", s.Gateway.String())
	if s.Type == hcloud.NetworkSubnetTypeVSwitch {
		d.Set("vswitch_id", s.VSwitchID)
	}
}

func generateNetworkSubnetID(network *hcloud.Network, ipRange string) string {
	return fmt.Sprintf("%d-%s", network.ID, ipRange)
}

// ParseSubnetID parses the faux subnet ID we from s.
//
// The faux subnet ID is created by the hcloud_network_subnet resource
// during creation. Using this method it can be read from the state and
// used in the implementation of other resources.
func ParseSubnetID(s string) (int64, *net.IPNet, error) {
	if s == "" {
		return 0, nil, errInvalidNetworkSubnetID
	}
	parts := strings.SplitN(s, "-", 2)
	if len(parts) != 2 {
		return 0, nil, errInvalidNetworkSubnetID
	}

	networkID, err := util.ParseID(parts[0])
	if err != nil {
		return 0, nil, errInvalidNetworkSubnetID
	}

	_, ipRange, err := net.ParseCIDR(parts[1])
	if ipRange == nil || err != nil {
		return 0, nil, errInvalidNetworkSubnetID
	}

	return networkID, ipRange, nil
}

var errInvalidNetworkSubnetID = errors.New("invalid network subnet id")

// lookupNetworkSubnetID parses the terraform network subnet record id and return the network and subnet
//
// id format: <network id>-<ip range>
// Examples:
// 123-192.168.100.1/32 (network subnet of network 123 with the ip range 192.168.100.1/32)
func lookupNetworkSubnetID(ctx context.Context, terraformID string, client *hcloud.Client) (*hcloud.Network, hcloud.NetworkSubnet, error) {
	networkID, ipRange, err := ParseSubnetID(terraformID)
	if err != nil {
		return nil, hcloud.NetworkSubnet{}, err
	}
	network, _, err := client.Network.GetByID(ctx, networkID)
	if err != nil {
		return nil, hcloud.NetworkSubnet{}, err
	}
	if network == nil {
		return nil, hcloud.NetworkSubnet{}, errInvalidNetworkSubnetID
	}
	for _, sn := range network.Subnets {
		if sn.IPRange.String() == ipRange.String() {
			return network, sn, nil
		}
	}
	return nil, hcloud.NetworkSubnet{}, nil
}
