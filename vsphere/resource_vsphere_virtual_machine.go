package vsphere

import (
	"fmt"
	"log"

	"github.com/hashicorp/terraform/helper/schema"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/vim25/mo"
)

func resourceVSphereVirtualMachine() *schema.Resource {
	return &schema.Resource{
		Create: resourceVSphereVirtualMachineCreate,
		Read:   resourceVSphereVirtualMachineRead,
		Delete: resourceVSphereVirtualMachineDelete,

		Schema: map[string]*schema.Schema{
			"name": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},

			"template": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},

			"datacenter": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
			},

			"cluster": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
			},

			"datastore": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
			},

			"vcpu": &schema.Schema{
				Type:     schema.TypeInt,
				Optional: true,
			},

			"memory": &schema.Schema{
				Type:     schema.TypeInt,
				Optional: true,
			},

			"gateway": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
			},

			"domain": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
			},

			"network_interface": &schema.Schema{
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"device_name": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},

						"label": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},

						"ip_address": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},

						"subnet_mask": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
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
	}

	if v := d.Get("datacenter"); v != nil {
		vm.Datacenter = d.Get("datacenter").(string)
	}

	if v := d.Get("cluster"); v != nil {
		vm.Cluster = d.Get("cluster").(string)
	}

	if v := d.Get("datastore"); v != nil {
		vm.Datastore = d.Get("datastore").(string)
	}

	if v := d.Get("vcpu"); v != nil {
		vm.VCPU = d.Get("vcpu").(int)
	}

	if v := d.Get("memory"); v != nil {
		vm.MemoryMB = int64(d.Get("memory").(int))
	}

	if v := d.Get("gateway"); v != nil {
		vm.Gateway = d.Get("gateway").(string)
	}

	if v := d.Get("domain"); v != nil {
		vm.Domain = d.Get("domain").(string)
	}

	if v := d.Get("network_interface"); v != nil {
		networksCount := d.Get("network_interface.#").(int)
		networks := make([]NetworkInterface, networksCount)
		for i := 0; i < networksCount; i++ {
			prefix := fmt.Sprintf("network_interface.%d", i)
			networks[i].DeviceName = d.Get(prefix + ".device_name").(string)
			networks[i].Label = d.Get(prefix + ".label").(string)
			networks[i].IPAddress = d.Get(prefix + ".ip_address").(string)
			networks[i].SubnetMask = d.Get(prefix + ".subnet_mask").(string)
		}
		vm.NetworkInterfaces = networks
		log.Printf("[DEBUG] network_interface init: %v", networks)
	}

	err := vm.RunVirtualMachine(client)
	if err != nil {
		return fmt.Errorf("error: %s", err)
	}
	d.SetId(vm.Name)
	log.Printf("[INFO] Created virtual machine: %s", d.Id())

	return resourceVSphereVirtualMachineRead(d, meta)
}

func resourceVSphereVirtualMachineRead(d *schema.ResourceData, meta interface{}) error {
	var dc *govmomi.Datacenter
	var err error

	client := meta.(*govmomi.Client)
	finder := find.NewFinder(client, true)

	if v := d.Get("datacenter"); v != nil {
		dc, err = finder.Datacenter(d.Get("datacenter").(string))
		if err != nil {
			return err
		}
	} else {
		dc, err = finder.DefaultDatacenter()
		if err != nil {
			return err
		}
	}

	d.Set("datacenter", dc)
	dcFolders, err := dc.Folders()
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

	err = client.Properties(vm.Reference(), []string{"summary"}, &mvm)

	d.Set("memory", mvm.Summary.Config.MemorySizeMB)
	d.Set("cpu", mvm.Summary.Config.NumCpu)

	return nil
}

func resourceVSphereVirtualMachineDelete(d *schema.ResourceData, meta interface{}) error {
	var dc *govmomi.Datacenter
	var err error

	client := meta.(*govmomi.Client)
	finder := find.NewFinder(client, true)
	log.Printf("[INFO] Deleting virtual machine: %s", d.Id())

	if v := d.Get("datacenter"); v != nil {
		dc, err = finder.Datacenter(d.Get("datacenter").(string))
		if err != nil {
			return err
		}
	} else {
		dc, err = finder.DefaultDatacenter()
		if err != nil {
			return err
		}
	}

	finder.SetDatacenter(dc)
	d.Set("datacenter", dc)
	dcFolders, err := dc.Folders()
	if err != nil {
		return fmt.Errorf("error %s", err)
	}

	vmRef, err := client.SearchIndex().FindChild(dcFolders.VmFolder, d.Get("name").(string))
	if err != nil {
		return fmt.Errorf("error %s", err)
	}

	vm := govmomi.NewVirtualMachine(client, vmRef.Reference())
	_, err = vm.PowerOff()
	if err != nil {
		return fmt.Errorf("error %s", err)
	}

	_, err = vm.Destroy()
	if err != nil {
		return fmt.Errorf("error %s", err)
	}

	d.SetId("")
	return nil
}
