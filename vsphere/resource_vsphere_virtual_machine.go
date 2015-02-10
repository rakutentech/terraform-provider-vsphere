package vsphere

import (
	"fmt"
	"log"

	"github.com/hashicorp/terraform/helper/schema"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
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
			},

			"datacenter": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
			},

			"cluster": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},

			"datastore": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},

			"template": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},

			"vcpu": &schema.Schema{
				Type:     schema.TypeInt,
				Optional: true,
			},

			"memory": &schema.Schema{
				Type:     schema.TypeInt,
				Optional: true,
			},

			"memory_reservation": &schema.Schema{
				Type:     schema.TypeInt,
				Optional: true,
			},

			"gateway": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
			},

			"network": &schema.Schema{
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},

						"label": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},

						"address": &schema.Schema{
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
		Name:       d.Get("name").(string),
		Datacenter: d.Get("datacenter").(string),
		Cluster:    d.Get("cluster").(string),
		Datastore:  d.Get("datastore").(string),
		VCPU:       d.Get("vcpu").(int),
		MemoryMB:   int64(d.Get("memory").(int)),
		Template:   d.Get("template").(string),
		Gateway:    d.Get("gateway").(string),
	}

	if v := d.Get("memory_reservation"); v != nil {
		vm.MemoryReservation = int64(d.Get("memory_reservation").(int))
	}

	if v := d.Get("iops"); v != nil {
		vm.IOPS = d.Get("iops").(int)
	}

	networksCount := d.Get("network.#").(int)
	networks := make([]NetworkInterface, networksCount)
	for i := 0; i < networksCount; i++ {
		prefix := fmt.Sprintf("network.%d", i)
		networks[i].Name = d.Get(prefix + ".name").(string)
		networks[i].Label = d.Get(prefix + ".label").(string)
		networks[i].IPAddress = d.Get(prefix + ".address").(string)
		networks[i].SubnetMask = d.Get(prefix + ".subnet_mask").(string)
	}
	vm.Networks = networks
	log.Printf("[DEBUG] network init: %v", networks)

	err := vm.RunVirtualMachine(client)
	if err != nil {
		return fmt.Errorf("error: %s", err)
	}
	d.SetId(vm.Name)
	d.Set("name", vm.Name)
	d.Set("datacenter", vm.Datacenter)
	d.Set("cluster", vm.Cluster)
	d.Set("datastore", vm.Datastore)

	return nil
}

func resourceVSphereVirtualMachineRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*govmomi.Client)

	vm := vm.RetriveVirtualMachine(client)

	return nil
}

func resourceVSphereVirtualMachineUpdate(d *schema.ResourceData, meta interface{}) error {
	return nil
}

func resourceVSphereVirtualMachineDelete(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*govmomi.Client)
	log.Printf("[INFO] Deleting virtual machine: %s, %s", d.Get("name").(string), d.Id())

	finder := find.NewFinder(client, true)
	dc, err := finder.Datacenter(d.Get("datacenter").(string))
	if err != nil {
		return fmt.Errorf("error %s", err)
	}

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
