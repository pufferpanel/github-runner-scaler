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

## Integrating to get JIT tokens

The next challenge for now is to just work out how to generate the tokens we need to 
create our own runners on demand. The tokens only last for an hour, but it probably
will be easier to just constantly request them.

Also learned the Google library for GitHub includes the stuff we need for the web server,
so I do not need to manually validate SHA256 or all of that. We should use this instead.

We will use the JIT config, which should just auto-drop into the runner configs and run
directly, so we don't need to call the crazy scripts as much. But we have to see.

Further research has shown that the JIT is only used when you call run.sh for a runner.
This isn't such a big deal, except not 100% sure how we'd be able to actually execute 
this well.

We could potentially just SSH into the server and execute it. This would give the "manager"
access to pull the logs as it'd have them directly. However, getting the IP for the VM
would be tricky. Because qemu-guest-agent is in theory installed, we should be able to
pull it from there.

This may end up also being a case for vendor-data to be used instead. 

The easier option at this stage is using SSH to do the configurations.
We will update the cloud-init to just generate our user and place the key. Then, use SSH
to remove in and set up and run the runner. We will use the SFTP client to drop our config
into place from the GitHub API and pass that to the runner. We'll log all of this so we can
accurately see what is going on.
The SSH connection will have to plan for a possible reboot when cloudinit does it. But, 
once we can actually SSH in, we should be configured correctly. We'll basically need a
timeout system where we need to SSH in within 5 minutes. If we do, we're fine. If we don't,
the VM should be considered trash.

https://dragonpit.rift.haus:8006/api2/json/nodes/dragonpit/qemu/107/agent/network-get-interfaces
Example URL

This gets the "qemu-guest-agent" stuff, so we can get the IPs that we need to look for here.
The issue with this is going to be which interface. Fortunately, network MACs are available.
We can use the MAC from the config and against this endpoint to find which IP we need to be
using to access the server.

So, after some thought, we're going to actually change this up. The template that we will run
with packer will copy our SSH key in directly and create the user. This way, all we need to
do is copy over the config, extra, then run the binary.

Caveat, this requires the builder to now have the key, but that's not much of an issue.

We will also disable the cloudflare tunnel for now. We don't need it for purposes of testing
until we actually need GitHub to give us enough data.

## Getting VMs working (2025-06-29)

After a few days of looking at the VMs, I've rebuilt the packer file to better align with using
the variables and configs. Not sure if this solves anything, but it made the VMs at least boot
correctly. I still needed to enable cloud-init so our interface would resolve an IP, but so far
the API calls to get the networking information work. We just needed to ensure the SSH key
authentication actually works as we wanted.

It turned out the SSH key that was being deployed wasn't what was configured. But, the SSH client
once told the correct key does get in, and can run commands. The logger wasn't best because of
batches including line-breaks, so we had to change to the regular Scanner to see if that makes 
better results.

The next step to solve is deleting the VM. Right now we're doing this manually, so it's a bit
annoying to do. We're just going to make this a defer action on creation, because we already
are using SSH to trigger the agent anyway. The idea was we didn't need to directly exist the
entire time, but that can be a future solution.

To delete, we're going to stop it, wait for it to stop (hopefully, this is a plug pull, so it
should be under a minute), then send a delete op. If stuff errors, we'll pretend it worked and
just move on. There's not much recovery at this stage.

Now the next trick is getting the GitHub config right. The caveat is the deleter actually ran
before the error was logged. That had to be moved around, but now it's going to log in the right
order.

https://docs.github.com/en/actions/how-tos/security-for-github-actions/security-guides/security-hardening-for-github-actions#using-just-in-time-runners

Finding this documentation about how to use the JIT config is annoying. It wasn't obvious, and
I ended up eventually finding it through a blog post. That's not ideal. Also, names are unique,
so if the runner config is generated, but we have issues starting it, the runner needs to be
removed from the organization. And this now works.

We have a working scaling system. In theory. This does run the jobs, and self-deletes after it
is done. The caveat is this isn't specific about which job it wants to use, so the ids don't
quite make sense.

## Changing ids

So, after the above was completed, what we've learned is that even though we do get the workflow
id, that's not actually what might run on our instance. So, any "tracking" that we are doing
doesn't really offer the value that we hoped for. So, we might just change this to use a GUID
so we just spin up a few and let them go. We could track the number of jobs queued up. Which,
this does work, but if we get out of sync, the VMs won't be correct. 

There might be an alterative, but it requires research. We could change that to a polling system,
where we simply poll GitHub and see if there are jobs queued. This isn't great, but it would mean
if we get out of sync, we're not completely stuck. If we assume we keep the redis data in sync,
then we don't need this. But, time will tell. This isn't exactly a perfect science. ARC is a
giant thing, so I can see why this isn't fun to solve.

The logging has been updated to store the full job log (only the actual SSH connection) to a file
with the run id. This means we can get the log of the job at least. Hooking this up to save the
entire log for the process is going to be next.

