# Exporting Inventory to Ansible

AeroLab supports exporting the cluster/client/agi inventory into ansible. To see the inventory in ansible format, simply run `aerolab inventory ansible`.

## Recommended method: Dynamic inventory

Running `aerolab showcommands` will install a command called `aerolab-ansible`. Ansible can then be used with dynamic inventory as so:

```bash
ansible-playbook -i aerolab-ansible playbook.yaml
```

## Dynamic inventory without installation

Alternatively, to avoid installing the command, run `ln -s ./aerolab ./aerolab-ansible` and then use `ansible-playbook -i ./aerolab-ansible playbook.yaml`

## Footnote

The concept and implementation are explained [on the ansible website](https://docs.ansible.com/ansible/latest/dev_guide/developing_inventory.html#developing-inventory-scripts).
