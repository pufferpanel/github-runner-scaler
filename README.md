# GitHub Runner Scaler

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
- Worker creates a VM with a name with a specific prefix and the job id cloned from
an existing VM template (ideally)
- Workflow starts the VM
- GitHub sends a "workflow_job.completed" event when the job is completed
- API consumes the event, writes the id to a different queue Redis
- Worker waits for an id to be posted to redis, pulls it
- Worker stops the VM
- Worker deletes the VM

# Resources

https://pve.proxmox.com/wiki/Proxmox_VE_API

https://pve.proxmox.com/pve-docs/api-viewer/

https://docs.github.com/en/actions/hosting-your-own-runners/managing-self-hosted-runners/autoscaling-with-self-hosted-runners

