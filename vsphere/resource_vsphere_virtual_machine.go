package vsphere

import (
	"fmt"
	"log"

	"github.com/hashicorp/terraform/helper/schema"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/mo"
	"golang.org/x/net/context"
)

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

			"template": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: false,
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

			"datastore": &schema.Schema{
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
			},

			"time_zone": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: false,
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
							ForceNew: false,
						},

						"subnet_mask": &schema.Schema{
							Type:     schema.TypeString,
							Optional: true,
							ForceNew: false,
						},
					},
				},
			},
		},
	}
}

func resourceVSphereVirtualMachineCreate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*govmomi.Client)

	vm := VirtualMachine{
		Name:     d.Get("name").(string),
		Template: d.Get("template").(string),
		VCPU:     d.Get("vcpu").(int),
		MemoryMB: int64(d.Get("memory").(int)),
	}

	if v := d.Get("datacenter"); v != nil {
		vm.Datacenter = d.Get("datacenter").(string)
	}

	if v := d.Get("cluster"); v != nil {
		vm.Cluster = d.Get("cluster").(string)
	}

	if v := d.Get("resource_pool"); v != nil {
		vm.ResourcePool = d.Get("resource_pool").(string)
	}

	if v := d.Get("datastore"); v != nil {
		vm.Datastore = d.Get("datastore").(string)
	}

	if v := d.Get("gateway"); v != nil {
		vm.Gateway = d.Get("gateway").(string)
	}

	if v := d.Get("domain"); v != nil {
		vm.Domain = d.Get("domain").(string)
	}

	if v := d.Get("time_zone"); v != nil {
		vm.TimeZone = d.Get("time_zone").(string)
	}

	dns_suffix := d.Get("dns_suffix.#").(int)
	if dns_suffix > 0 {
		vm.DNSSuffixes = make([]string, 0, dns_suffix)
		for i := 0; i < dns_suffix; i++ {
			s := fmt.Sprintf("dns_suffix.%d", i)
			vm.DNSSuffixes = append(vm.DNSSuffixes, d.Get(s).(string))
		}
	}

	dns_server := d.Get("dns_server.#").(int)
	if dns_server > 0 {
		vm.DNSServers = make([]string, 0, dns_server)
		for i := 0; i < dns_server; i++ {
			s := fmt.Sprintf("dns_server.%d", i)
			vm.DNSServers = append(vm.DNSServers, d.Get(s).(string))
		}
	}

	networksCount := d.Get("network_interface.#").(int)
	networks := make([]NetworkInterface, networksCount)
	for i := 0; i < networksCount; i++ {
		prefix := fmt.Sprintf("network_interface.%d", i)
		networks[i].Label = d.Get(prefix + ".label").(string)
		if v := d.Get(prefix + ".ip_address"); v != nil {
			networks[i].IPAddress = d.Get(prefix + ".ip_address").(string)
			networks[i].SubnetMask = d.Get(prefix + ".subnet_mask").(string)
		}
	}
	vm.NetworkInterfaces = networks
	log.Printf("[DEBUG] network_interface init: %v", networks)

	err := vm.deployVirtualMachine(client)
	if err != nil {
		return fmt.Errorf("error: %s", err)
	}
	d.SetId(vm.Name)
	log.Printf("[INFO] Created virtual machine: %s", d.Id())

	return resourceVSphereVirtualMachineRead(d, meta)
}

func resourceVSphereVirtualMachineRead(d *schema.ResourceData, meta interface{}) error {
	var dc *object.Datacenter
	var err error

	client := meta.(*govmomi.Client)
	finder := find.NewFinder(client.Client, true)
	dc, err = getDatacenter(finder, d.Get("datacenter").(string))
	if err != nil {
		return err
	}

	d.Set("datacenter", dc)
	dcFolders, err := dc.Folders(context.TODO())
	if err != nil {
		return err
	}

	vm, err := getVirtualMachine(client, dcFolders.VmFolder, d.Get("name").(string))
	if err != nil {
		log.Printf("[ERROR] Virtual machine not found: %s", d.Get("name").(string))
		d.SetId("")
		return nil
	}

	var mvm mo.VirtualMachine

	collector := property.DefaultCollector(client.Client)
	err = collector.RetrieveOne(context.TODO(), vm.Reference(), []string{"summary"}, &mvm)

	d.Set("memory", mvm.Summary.Config.MemorySizeMB)
	d.Set("cpu", mvm.Summary.Config.NumCpu)

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
	dc, err = getDatacenter(finder, d.Get("datacenter").(string))
	if err != nil {
		return err
	}

	d.Set("datacenter", dc)
	dcFolders, err := dc.Folders(context.TODO())
	if err != nil {
		return err
	}

	vm, err := getVirtualMachine(client, dcFolders.VmFolder, d.Get("name").(string))
	if err != nil {
		return err
	}

	log.Printf("[INFO] Deleting virtual machine: %s", d.Id())

	_, err = vm.PowerOff(context.TODO())
	if err != nil {
		return err
	}

	_, err = vm.Destroy(context.TODO())
	if err != nil {
		return err
	}

	d.SetId("")
	return nil
}
