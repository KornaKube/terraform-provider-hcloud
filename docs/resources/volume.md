---
page_title: "Hetzner Cloud: hcloud_volume"
description: |-
  Provides a Hetzner Cloud volume resource to manage volumes.
---

# hcloud_volume

Provides a Hetzner Cloud volume resource to manage volumes.

## Example Usage

```terraform
resource "hcloud_server" "node1" {
  name        = "node1"
  image       = "debian-11"
  server_type = "cx22"
}

resource "hcloud_volume" "master" {
  name      = "volume1"
  size      = 50
  server_id = hcloud_server.node1.id
  automount = true
  format    = "ext4"
}
```

## Argument Reference

- `name` - (Required, string) Name of the volume to create (must be unique per project).
- `size` - (Required, int) Size of the volume (in GB).
- `labels` - (Optional, map) User-defined labels (key-value pairs).
- `server_id` - (Optional, int) Server to attach the Volume to, not allowed if location argument is passed.
- `location` - (Optional, string) The location name of the volume to create, not allowed if server_id argument is passed. See the [Hetzner Docs](https://docs.hetzner.com/cloud/general/locations/#what-locations-are-there) for more details about locations.
- `automount` - (Optional, bool) Automount the volume upon attaching it (server_id must be provided).
- `format` - (Optional, string) Format volume after creation. `xfs` or `ext4`
- `delete_protection` - (Optional, bool) Enable or disable delete protection. See ["Delete Protection"](../index.html.markdown#delete-protection) in the Provider Docs for details.

**Note:** When you want to attach multiple volumes to a server, please use the `hcloud_volume_attachment` resource and the `location` argument instead of the `server_id` argument.

## Attributes Reference

- `id` - (int) Unique ID of the volume.
- `name` - (string) Name of the volume.
- `size` - (int) Size of the volume.
- `location` - (string) The location name. See the [Hetzner Docs](https://docs.hetzner.com/cloud/general/locations/#what-locations-are-there) for more details about locations.
- `server_id` - (Optional, int) Server ID the volume is attached to
- `labels` - (map) User-defined labels (key-value pairs).
- `linux_device` - (string) Device path on the file system for the Volume.
- `delete_protection` - (bool) Whether delete protection is enabled.

## Import

Volumes can be imported using their `id`:

```shell
terraform import hcloud_volume.example "$VOLUME_ID"
```
