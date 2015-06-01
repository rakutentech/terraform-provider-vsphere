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
							ForceNew: false,
						},

						"subnet_mask": &schema.Schema{
							Type:     schema.TypeString,
							Optional: true,
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

	if v := d.Get("datacenter"); v != nil {
		vm.datacenter = d.Get("datacenter").(string)
	}

	if v := d.Get("cluster"); v != nil {
		vm.cluster = d.Get("cluster").(string)
	}

	if v := d.Get("resource_pool"); v != nil {
		vm.resourcePool = d.Get("resource_pool").(string)
	}

	if v := d.Get("gateway"); v != nil {
		vm.gateway = d.Get("gateway").(string)
	}

	if v := d.Get("domain"); v != nil {
		vm.domain = d.Get("domain").(string)
	}

	if v := d.Get("time_zone"); v != nil {
		vm.timeZone = d.Get("time_zone").(string)
	}

	dns_suffix := d.Get("dns_suffix.#").(int)
	if dns_suffix > 0 {
		vm.dnsSuffixes = make([]string, 0, dns_suffix)
		for i := 0; i < dns_suffix; i++ {
			s := fmt.Sprintf("dns_suffix.%d", i)
			vm.dnsSuffixes = append(vm.dnsSuffixes, d.Get(s).(string))
		}
	} else {
		vm.dnsSuffixes = []string{"vsphere.local"}
	}

	dns_server := d.Get("dns_server.#").(int)
	if dns_server > 0 {
		vm.dnsServers = make([]string, 0, dns_server)
		for i := 0; i < dns_server; i++ {
			s := fmt.Sprintf("dns_server.%d", i)
			vm.dnsServers = append(vm.dnsServers, d.Get(s).(string))
		}
	} else {
		vm.dnsServers = []string{
			"8.8.8.8",
			"8.8.4.4",
		}
	}

	networksCount := d.Get("network_interface.#").(int)
	networks := make([]networkInterface, networksCount)
	for i := 0; i < networksCount; i++ {
		prefix := fmt.Sprintf("network_interface.%d", i)
		networks[i].label = d.Get(prefix + ".label").(string)
		if v := d.Get(prefix + ".ip_address"); v != nil {
			networks[i].ipAddress = d.Get(prefix + ".ip_address").(string)
			networks[i].subnetMask = d.Get(prefix + ".subnet_mask").(string)
		}
	}
	vm.networkInterfaces = networks
	log.Printf("[DEBUG] network_interface init: %v", networks)

	diskCount := d.Get("disk.#").(int)
	disks := make([]hardDisk, diskCount)
	for i := 0; i < diskCount; i++ {
		prefix := fmt.Sprintf("disk.%d", i)
		if i == 0 {
			if v := d.Get(prefix + ".template"); v != "" {
				vm.template = d.Get(prefix + ".template").(string)
			} else {
				if v := d.Get(prefix + ".size"); v != "" {
					disks[i].size = int64(d.Get(prefix + ".size").(int))
				} else {
					return fmt.Errorf("If template argument is not specified, size argument is required.")
				}
			}
			if v := d.Get(prefix + ".datastore"); v != "" {
				vm.datastore = d.Get(prefix + ".datastore").(string)
			}
		} else {
			if v := d.Get(prefix + ".size"); v != "" {
				disks[i].size = int64(d.Get(prefix + ".size").(int))
			} else {
				return fmt.Errorf("Size argument is required.")
			}
		}
		if v := d.Get(prefix + ".iops"); v != "" {
			disks[i].iops = int64(d.Get(prefix + ".iops").(int))
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
	d.SetId(vm.name)
	log.Printf("[INFO] Created virtual machine: %s", d.Id())

	return resourceVSphereVirtualMachineRead(d, meta)
}

func resourceVSphereVirtualMachineRead(d *schema.ResourceData, meta interface{}) error {
	var dc *object.Datacenter
	var err error

	client := meta.(*govmomi.Client)
	finder := find.NewFinder(client.Client, true)

	if d.Get("datacenter").(string) != "" {
		dc, err = finder.Datacenter(context.TODO(), d.Get("datacenter").(string))
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
	err = collector.RetrieveOne(context.TODO(), vm.Reference(), []string{"summary"}, &mvm)

	d.Set("datacenter", dc)
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

	if d.Get("datacenter").(string) != "" {
		dc, err = finder.Datacenter(context.TODO(), d.Get("datacenter").(string))
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
