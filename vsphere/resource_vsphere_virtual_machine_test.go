package vsphere

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/terraform"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"golang.org/x/net/context"
)

func TestAccVSphereVirtualMachine_Basic(t *testing.T) {
	var vm virtualMachine
	name := os.Getenv("VSPHERE_VM_NAME")
	datacenter := os.Getenv("VSPHERE_DATACENTER")
	cluster := os.Getenv("VSPHERE_CLUSTER")
	datastore := os.Getenv("VSPHERE_DATASTORE")
	template := os.Getenv("VSPHERE_TEMPLATE")
	label := os.Getenv("VSPHERE_NETWORK_LABEL")

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckVSphereVirtualMachineDestroy,
		Steps: []resource.TestStep{
			resource.TestStep{
				Config: fmt.Sprintf(
					testAccCheckVSphereVirtualMachineConfig_basic,
					name,
					datacenter,
					cluster,
					label,
					datastore,
					template,
				),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckVSphereVirtualMachineExists("vsphere_virtual_machine.foobar", &vm),
					//		testAccCheckVSphereVirtualMachineAttributes(&vm),
					resource.TestCheckResourceAttr(
						"vsphere_virtual_machine.foobar", "name", name),
				),
			},
		},
	})
}

func testAccCheckVSphereVirtualMachineDestroy(s *terraform.State) error {
	client := testAccProvider.Meta().(*govmomi.Client)
	finder := find.NewFinder(client.Client, true)

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "vsphere_virtual_machine" {
			continue
		}

		dc, err := finder.Datacenter(context.TODO(), rs.Primary.Attributes["datacenter"])
		if err != nil {
			return fmt.Errorf("error %s", err)
		}

		dcFolders, err := dc.Folders(context.TODO())
		if err != nil {
			return fmt.Errorf("error %s", err)
		}

		_, err = object.NewSearchIndex(client.Client).FindChild(context.TODO(), dcFolders.VmFolder, rs.Primary.Attributes["name"])
		if err == nil {
			return fmt.Errorf("Record still exists")
		}
	}

	return nil
}

func testAccCheckVSphereVirtualMachineExists(n string, vm *virtualMachine) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("No ID is set")
		}

		client := testAccProvider.Meta().(*govmomi.Client)
		finder := find.NewFinder(client.Client, true)

		dc, err := finder.Datacenter(context.TODO(), rs.Primary.Attributes["datacenter"])
		if err != nil {
			return fmt.Errorf("error %s", err)
		}

		dcFolders, err := dc.Folders(context.TODO())
		if err != nil {
			return fmt.Errorf("error %s", err)
		}

		_, err = object.NewSearchIndex(client.Client).FindChild(context.TODO(), dcFolders.VmFolder, rs.Primary.Attributes["name"])
		/*
			vmRef, err := client.SearchIndex().FindChild(dcFolders.VmFolder, rs.Primary.Attributes["name"])
			if err != nil {
				return fmt.Errorf("error %s", err)
			}

			found := govmomi.NewVirtualMachine(client, vmRef.Reference())
			fmt.Printf("%v", found)

			if found.Name != rs.Primary.ID {
				return fmt.Errorf("Instance not found")
			}
			*instance = *found
		*/

		*vm = virtualMachine{
			name: rs.Primary.ID,
		}

		return nil
	}
}

const testAccCheckVSphereVirtualMachineConfig_basic = `
resource "vsphere_virtual_machine" "foobar" {
    name = "%s"
    datacenter = "%s"
    cluster = "%s"
    vcpu = 2
    memory = 4096
    gateway = "192.168.0.254"
    network_interface {
        label = "%s"
        ip_address = "192.168.0.10"
        subnet_mask = "255.255.255.0"
    }
    disk {
        datastore = "%s"
        template = "%s"
        iops = 500
    }
    disk {
        size = 1
        iops = 500
    }
}
`
