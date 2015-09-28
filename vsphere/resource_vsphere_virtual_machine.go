package vsphere

import (
	"fmt"
	"log"
	"net"
	"time"

	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/mo"
	"golang.org/x/net/context"
)

var DefaultDNSSuffixes = []string{
	"vsphere.local",
}

var DefaultDNSServers = []string{
	"8.8.8.8",
	"8.8.4.4",
}

func resourceVSphereVirtualMachine() *schema.Resource {
	return &schema.Resource{
		Create: resourceVSphereVirtualMachineCreate,
		Read:   resourceVSphereVirtualMachineRead,
		Update: resourceVSphereVirtualMachineUpdate,
		Delete: resourceVSphereVirtualMachineDelete,

		Schema: map[string]*schema.Schema{
			"name": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"vcpu": &schema.Schema{
				Type:     schema.TypeInt,
				Required: true,
				ForceNew: false,
			},

			"memory": &schema.Schema{
				Type:     schema.TypeInt,
				Required: true,
				ForceNew: false,
			},

			"datacenter": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: false,
			},

			"cluster": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: false,
			},

			"resource_pool": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: false,
			},

			"gateway": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: false,
			},

			"domain": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: false,
				Default:  "vsphere.local",
			},

			"time_zone": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: false,
				Default:  "Etc/UTC",
			},

			"dns_suffix": &schema.Schema{
				Type:     schema.TypeList,
				Optional: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
				ForceNew: false,
			},

			"dns_server": &schema.Schema{
				Type:     schema.TypeList,
				Optional: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
				ForceNew: false,
			},

			"network_interface": &schema.Schema{
				Type:     schema.TypeList,
				Required: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"label": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
							ForceNew: false,
						},

						"ip_address": &schema.Schema{
							Type:     schema.TypeString,
							Optional: true,
							Computed: true,
							ForceNew: false,
						},

						"subnet_mask": &schema.Schema{
							Type:     schema.TypeString,
							Optional: true,
							Computed: true,
							ForceNew: false,
						},

						"adapter_type": &schema.Schema{
							Type:     schema.TypeString,
							Optional: true,
							ForceNew: false,
						},
					},
				},
			},

			"disk": &schema.Schema{
				Type:     schema.TypeList,
				Required: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"template": &schema.Schema{
							Type:     schema.TypeString,
							Optional: true,
							ForceNew: false,
						},

						"datastore": &schema.Schema{
							Type:     schema.TypeString,
							Optional: true,
							ForceNew: false,
						},

						"size": &schema.Schema{
							Type:     schema.TypeInt,
							Optional: true,
							ForceNew: false,
						},

						"iops": &schema.Schema{
							Type:     schema.TypeInt,
							Optional: true,
							ForceNew: false,
						},
					},
				},
			},

			"boot_delay": &schema.Schema{
				Type:     schema.TypeInt,
				Optional: true,
			},
		},
	}
}

func resourceVSphereVirtualMachineCreate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*govmomi.Client)

	vm := virtualMachine{
		name:     d.Get("name").(string),
		vcpu:     d.Get("vcpu").(int),
		memoryMb: int64(d.Get("memory").(int)),
	}

	if v, ok := d.GetOk("datacenter"); ok {
		vm.datacenter = v.(string)
	}

	if v, ok := d.GetOk("cluster"); ok {
		vm.cluster = v.(string)
	}

	if v, ok := d.GetOk("resource_pool"); ok {
		vm.resourcePool = v.(string)
	}

	if v, ok := d.GetOk("gateway"); ok {
		vm.gateway = v.(string)
	}

	if v, ok := d.GetOk("domain"); ok {
		vm.domain = v.(string)
	}

	if v, ok := d.GetOk("time_zone"); ok {
		vm.timeZone = v.(string)
	}

	dns_suffix := d.Get("dns_suffix.#").(int)
	if dns_suffix > 0 {
		vm.dnsSuffixes = make([]string, 0, dns_suffix)
		for i := 0; i < dns_suffix; i++ {
			s := fmt.Sprintf("dns_suffix.%d", i)
			vm.dnsSuffixes = append(vm.dnsSuffixes, d.Get(s).(string))
		}
	} else {
		vm.dnsSuffixes = DefaultDNSSuffixes
	}

	dns_server := d.Get("dns_server.#").(int)
	if dns_server > 0 {
		vm.dnsServers = make([]string, 0, dns_server)
		for i := 0; i < dns_server; i++ {
			s := fmt.Sprintf("dns_server.%d", i)
			vm.dnsServers = append(vm.dnsServers, d.Get(s).(string))
		}
	} else {
		vm.dnsServers = DefaultDNSServers
	}

	networksCount := d.Get("network_interface.#").(int)
	networks := make([]networkInterface, networksCount)
	for i := 0; i < networksCount; i++ {
		prefix := fmt.Sprintf("network_interface.%d", i)
		networks[i].label = d.Get(prefix + ".label").(string)
		if v, ok := d.GetOk(prefix + ".ip_address"); ok {
			networks[i].ipAddress = v.(string)
		}
		if v, ok := d.GetOk(prefix + ".subnet_mask"); ok {
			networks[i].subnetMask = v.(string)
		}
	}
	vm.networkInterfaces = networks
	log.Printf("[DEBUG] network_interface init: %v", networks)

	diskCount := d.Get("disk.#").(int)
	disks := make([]hardDisk, diskCount)
	for i := 0; i < diskCount; i++ {
		prefix := fmt.Sprintf("disk.%d", i)
		if i == 0 {
			if v, ok := d.GetOk(prefix + ".template"); ok {
				vm.template = v.(string)
			} else {
				if v, ok := d.GetOk(prefix + ".size"); ok {
					disks[i].size = int64(v.(int))
				} else {
					return fmt.Errorf("If template argument is not specified, size argument is required.")
				}
			}
			if v, ok := d.GetOk(prefix + ".datastore"); ok {
				vm.datastore = v.(string)
			}
		} else {
			if v, ok := d.GetOk(prefix + ".size"); ok {
				disks[i].size = int64(v.(int))
			} else {
				return fmt.Errorf("Size argument is required.")
			}
		}
		if v, ok := d.GetOk(prefix + ".iops"); ok {
			disks[i].iops = int64(v.(int))
		}
	}
	vm.hardDisks = disks
	log.Printf("[DEBUG] disk init: %v", disks)

	if vm.template != "" {
		err := vm.deployVirtualMachine(client)
		if err != nil {
			return fmt.Errorf("error: %s", err)
		}
	} else {
		err := vm.createVirtualMachine(client)
		if err != nil {
			return fmt.Errorf("error: %s", err)
		}
	}

	if _, ok := d.GetOk("network_interface.0.ip_address"); !ok {
		if v, ok := d.GetOk("boot_delay"); ok {
			stateConf := &resource.StateChangeConf{
				Pending:    []string{"pending"},
				Target:     "active",
				Refresh:    waitForNetworkingActive(client, vm.datacenter, vm.name),
				Timeout:    600 * time.Second,
				Delay:      time.Duration(v.(int)) * time.Second,
				MinTimeout: 2 * time.Second,
			}

			_, err := stateConf.WaitForState()
			if err != nil {
				return fmt.Errorf("error: %s", err)
			}
		}
	}
	d.SetId(vm.name)
	log.Printf("[INFO] Created virtual machine: %s", d.Id())

	return resourceVSphereVirtualMachineRead(d, meta)
}

func resourceVSphereVirtualMachineRead(d *schema.ResourceData, meta interface{}) error {
	var dc *object.Datacenter
	var err error

	client := meta.(*govmomi.Client)
	finder := find.NewFinder(client.Client, true)

	if v, ok := d.GetOk("datacenter"); ok {
		dc, err = finder.Datacenter(context.TODO(), v.(string))
		if err != nil {
			return err
		}
	} else {
		dc, err = finder.DefaultDatacenter(context.TODO())
		if err != nil {
			return err
		}
	}

	finder = finder.SetDatacenter(dc)
	vm, err := finder.VirtualMachine(context.TODO(), d.Get("name").(string))
	if err != nil {
		log.Printf("[ERROR] Virtual machine not found: %s", d.Get("name").(string))
		d.SetId("")
		return nil
	}

	var mvm mo.VirtualMachine

	collector := property.DefaultCollector(client.Client)
	if err := collector.RetrieveOne(context.TODO(), vm.Reference(), []string{"guest", "summary", "datastore"}, &mvm); err != nil {
		log.Printf("[ERROR] %#v", err)
	}

	log.Printf("[DEBUG] %#v", dc)
	log.Printf("[DEBUG] %#v", mvm.Summary.Config)
	log.Printf("[DEBUG] %#v", mvm.Guest.Net)

	networkInterfaces := make([]map[string]interface{}, 0)
	for _, v := range mvm.Guest.Net {
		if v.DeviceConfigId >= 0 {
			log.Printf("[DEBUG] %#v", v.Network)
			networkInterface := make(map[string]interface{})
			networkInterface["label"] = v.Network
			if len(v.IpAddress) > 0 {
				log.Printf("[DEBUG] %#v", v.IpAddress[0])
				networkInterface["ip_address"] = v.IpAddress[0]

				m := net.CIDRMask(v.IpConfig.IpAddress[0].PrefixLength, 32)
				subnetMask := net.IPv4(m[0], m[1], m[2], m[3])
				networkInterface["subnet_mask"] = subnetMask.String()
				log.Printf("[DEBUG] %#v", subnetMask.String())
			}
			networkInterfaces = append(networkInterfaces, networkInterface)
		}
	}
	d.Set("network_interface", networkInterfaces)

	var rootDatastore string
	for _, v := range mvm.Datastore {
		var md mo.Datastore
		if err := collector.RetrieveOne(context.TODO(), v, []string{"name", "parent"}, &md); err != nil {
			log.Printf("[ERROR] %#v", err)
		}
		if md.Parent.Type == "StoragePod" {
			var msp mo.StoragePod
			if err := collector.RetrieveOne(context.TODO(), *md.Parent, []string{"name"}, &msp); err != nil {
				log.Printf("[ERROR] %#v", err)
			}
			rootDatastore = msp.Name
			log.Printf("[DEBUG] %#v", msp.Name)
		} else {
			rootDatastore = md.Name
			log.Printf("[DEBUG] %#v", md.Name)
		}
		break
	}

	d.Set("datacenter", dc)
	d.Set("memory", mvm.Summary.Config.MemorySizeMB)
	d.Set("cpu", mvm.Summary.Config.NumCpu)
	d.Set("datastore", rootDatastore)

	// Initialize the connection info
	d.SetConnInfo(map[string]string{
		"type": "ssh",
		"host": networkInterfaces[0]["ip_address"].(string),
	})

	return nil
}

func resourceVSphereVirtualMachineUpdate(d *schema.ResourceData, meta interface{}) error {
	return nil
}

func resourceVSphereVirtualMachineDelete(d *schema.ResourceData, meta interface{}) error {
	var dc *object.Datacenter
	var err error

	client := meta.(*govmomi.Client)
	finder := find.NewFinder(client.Client, true)

	if v, ok := d.GetOk("datacenter"); ok {
		dc, err = finder.Datacenter(context.TODO(), v.(string))
		if err != nil {
			return err
		}
	} else {
		dc, err = finder.DefaultDatacenter(context.TODO())
		if err != nil {
			return err
		}
	}

	finder = finder.SetDatacenter(dc)
	vm, err := finder.VirtualMachine(context.TODO(), d.Get("name").(string))
	if err != nil {
		return err
	}

	log.Printf("[INFO] Deleting virtual machine: %s", d.Id())

	task, err := vm.PowerOff(context.TODO())
	if err != nil {
		return err
	}

	err = task.Wait(context.TODO())
	if err != nil {
		return err
	}

	task, err = vm.Destroy(context.TODO())
	if err != nil {
		return err
	}

	err = task.Wait(context.TODO())
	if err != nil {
		return err
	}

	d.SetId("")
	return nil
}

func waitForNetworkingActive(client *govmomi.Client, datacenter, name string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		finder := find.NewFinder(client.Client, true)
		var dc *object.Datacenter
		var err error

		if datacenter != "" {
			dc, err = finder.Datacenter(context.TODO(), datacenter)
		} else {
			dc, err = finder.DefaultDatacenter(context.TODO())
		}

		if err != nil {
			log.Printf("[ERROR] %#v", err)
			return nil, "", err
		}

		finder = finder.SetDatacenter(dc)
		vm, err := finder.VirtualMachine(context.TODO(), name)
		if err != nil {
			log.Printf("[ERROR] %#v", err)
			return nil, "", err
		}

		var mvm mo.VirtualMachine
		collector := property.DefaultCollector(client.Client)
		if err := collector.RetrieveOne(context.TODO(), vm.Reference(), []string{"summary"}, &mvm); err != nil {
			log.Printf("[ERROR] %#v", err)
			return nil, "", err
		}

		if mvm.Summary.Guest.IpAddress != "" {
			log.Printf("[DEBUG] IP address with DHCP: %v", mvm.Summary.Guest.IpAddress)
			return mvm.Summary, "active", err
		} else {
			log.Printf("[DEBUG] Waiting for IP address")
			return nil, "pending", err
		}
	}
}
