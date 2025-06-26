# Blog

This represents the mad descent into this project. Rather than cluttering the readme,
this will serve as any "findings" I find during this endeavor. This is an append-only
type of document, this way the history is preserved. The readme isn't.

## Initial (2025-06-25)
(copied from the readme at the initial creation)

This is mainly a mad scientist project, but it's of value for us.

Because ARC (GitHub's recommended tool for scalable self-hosted runners)
uses Kubernetes rather than VMs, this has a hard limitation when you want
to start doing tasks that need an actual machine (or a VM). Docker-in-docker
also isn't the greatest with this as well.

So, here comes GRS.

This brainchild takes the concept of being able to scale runners, with the
power and true isolation of VMs. This way, we don't need to spend money on
GitHub runners when we can do it ourselves. This is valuable when you have
local specific services that can help speed up certain jobs (i.e. LanCache)
that are not as easy to deploy into a GitHub runner.

This is built up using the Proxmox API and using Proxmox as the host. When
a workflow is queued up that has a specific runner group label, this will
create a VM for that job and start it up. This works by cloning a VM which
is mostly pre-configured with all the software provided by GitHub using their
image repo.

The concept is that the VM is temporary. Once the job is done (which is another
event from GitHub), we will remove the VM responsible. This means hopefully VMs
are short-lived enough that the host isn't cluttered up.

This is not remotely ready for production use, but this is a fun project.

Flow:
- GitHub sends a "workflow_job.queued" event
- API consumes the event, writes the id to a queue in Redis
- Worker waits for an id to be posted to redis, pulls it
- Worker creates a VM with a name with a specific prefix and the job id cloned
  from an existing VM template (ideally)
- Workflow starts the VM
- GitHub sends a "workflow_job.completed" event when the job is completed
- API consumes the event, writes the id to a different queue Redis
- Worker waits for an id to be posted to redis, pulls it
- Worker stops the VM
- Worker deletes the VM

The template VM is based off the runner-images repo for Ubuntu. The existing image
script assume Azure though, so they cannot be directly used. We had to kill some
of the scripts and tasks in order for it to build within our ecosystem. However,
this isn't too trivial, as mainly it relates to storage. This image isn't deployed
publicly yet as the secret tokens haven't been removed from the script.

## Getting the VMs set up with GitHub (2025-06-26)

The next issues are getting the VM to register correctly with GitHub. Cloud-init
does not make the correct data available to configure the instance, so unless we go
and create an SSH connection to the VM, this is going to be difficult.

In the worse case, we have to manually generate the cloud-init script and deploy it
to Proxmox. This then can be attached to the new instance and used to load it. This
however is ugly. If we assume a single instance, we could manage it, but this feels
like a hack around this. But, we do what we must. On-ward mad science!

Runners do support unattended installs, which is what we need here. The VM includes
the runner by default, but it's a specific version. Based on their docs, we will have
to regenerate the template VM every now than then to keep this up to date. This would
avoid the (100) VMs updating the runner each time it runs. 

In theory, cloud-init will let us send in the data using a meta, so we don't need to
rebuild the actual init script, but just need to provide the new metadata. Cloud-init
doesn't have the best documentation... This is a mess to figure out. But we will give
this a try. 

Further research has indicated this is just not possible directly without direct access
to the Proxmox host.

https://forum.proxmox.com/threads/creating-snippets-using-pve-api.54081/

https://bugzilla.proxmox.com/show_bug.cgi?id=2208

https://gist.github.com/aw/ce460c2100163c38734a83e09ac0439a

We will have to manually SFTP the file into the server for using it. That's annoying...
But, that's something we can manage to do at least.

Steps are:
- Clone VM
- Add snippet to proxmox host
- Update cicustom with snippet
- Rebuild cloudinit image (?)
- Start VM

We still don't have the JIT token this actually needs solved yet, that's another step 
down the line.

------

After more work, the tool is able to properly "configure" things, however it doesn't
properly run CloudInit. This requires more research to see what's going on. While we
set the user and ssh key, these don't appear to be copied over correctly.

Oddly, our code isn't handling errors from the Proxmox API yet. We are getting a 400
when setting config stuff. This turned out to be my check was > 400... opps.

SSH Keys needs to be urlencoded for us to include it. This is more to help ensure that
we can SSH into the server if we need to debug what's going on. This isn't required at 
least.

Turns out it's just use a different function. That's now fixed.

But now, CloudInit doesn't appear to be running how we want. The VM isn't reconfiguring
as we need it to in order to add our key + create GitHub runner. We'll need to look into
what's going on more. The Proxmox calls at least work.