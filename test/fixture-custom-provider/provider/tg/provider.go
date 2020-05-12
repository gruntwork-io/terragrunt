package tg

import (
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/terraform"
)

func Provider() terraform.ResourceProvider {
	return &schema.Provider{
		DataSourcesMap: map[string]*schema.Resource{
			"tg_welcome": dataSourceWelcome(),
		},
	}
}

func dataSourceWelcome() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceWelcomeRead,

		Schema: map[string]*schema.Schema{
			"text": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func dataSourceWelcomeRead(d *schema.ResourceData, meta interface{}) error {
	d.SetId("exists")
	if err := d.Set("text", "Hello from custom provider!"); err != nil {
		return err
	}
	return nil
}
