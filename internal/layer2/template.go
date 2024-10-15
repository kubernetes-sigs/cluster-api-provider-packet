package layer2

const configTemplate = `
#cloud-config

package_update: true
package_upgrade: true
packages:
  - jq
  - vlan

write_files:
  - path: /var/lib/capi_network_settings/final_configuration.sh
    permissions: '0755'
    content: |
      #!/bin/bash
      set -euo pipefail

      echo "Running final configuration commands"
      apt-get update -qq
      apt-get install -y -qq jq vlan

      modprobe 8021q
      echo "8021q" >> /etc/modules

      # Generate the network configuration and append it to /etc/network/interfaces for each VLAN-tagged sub-interface.
      cat <<EOL >> /etc/network/interfaces
{{ range .VLANs }}
      auto {{ .PortName }}.{{ .Vxlan }}
      iface {{ .PortName }}.{{ .Vxlan }} inet static
        pre-up sleep 5
        address {{ .IPAddress }}
        netmask {{ .Netmask }}
      {{- if .Gateway }}
        gateway {{ .Gateway }}
      {{- end }}
        vlan-raw-device {{ .PortName }}
      {{- range .Routes }}
        up ip route add {{ .Destination }} via {{ .Gateway }}
      {{- end }}
{{ end }}
      EOL

      echo "VLAN configuration appended to /etc/network/interfaces."

      # Function to send user state events
      url="$(curl -sf https://metadata.platformequinix.com/metadata | jq -r .user_state_url)"

      # Function to send user state events
      send_user_state_event() {
          local state="$1"
          local code="$2"
          local message="$3"
          local data
          local max_retries=3
          local retry_count=0
          
          data=$(jq -n --arg state "$state" --arg code "$code" --arg message "$message" \
                '{state: $state, code: ($code | tonumber), message: $message}')

          while [ $retry_count -lt $max_retries ]; do
              # Make the POST request and capture the HTTP status code
              http_code=$(curl -s -o /dev/null -w "%{http_code}" -X POST -d "$data" "$url")

              echo "HTTP Status Code: $http_code"

              if [[ "$http_code" -ge 200 && "$http_code" -lt 300 ]]; then
                  echo "User state event sent successfully on attempt $((retry_count + 1))"
                  return 0
              else
                  echo "Warning: Received non-success status code: $http_code"
              fi

              retry_count=$((retry_count + 1))
              if [ $retry_count -lt $max_retries ]; then
                  echo "Retrying in 5 seconds..."
                  sleep 5
              fi
          done

          echo "Error: Failed to send user state event after $max_retries attempts"
          return 1
      }


      send_user_state_event running 1000 "network_configuration_started"

      # Restart networking to apply VLAN configurations
      echo "Restarting networking service..."
      systemctl restart networking

      # Wait for interfaces to be fully up
      echo "Waiting for interfaces to be up..."
      sleep 5

      # Verify network configuration
      verification_failed=false
{{ range .VLANs }}
      if ip addr show {{ .PortName }}.{{ .Vxlan }} | grep -q {{ .IPAddress }}; then
        echo "Configuration for VLAN {{ .Vxlan }} on {{ .PortName }} with IP {{ .IPAddress }} successful"
      else
        echo "Configuration for VLAN {{ .Vxlan }} on {{ .PortName }} with IP {{ .IPAddress }} failed" >&2
        verification_failed=true
      fi

      # Verify static routes
{{ range .Routes }}
      if ip route | grep -q "{{ .Destination }} via {{ .Gateway }}"; then
        echo "Static route to {{ .Destination }} via {{ .Gateway }} added successfully"
      else
        echo "Failed to add static route to {{ .Destination }} via {{ .Gateway }}" >&2
        verification_failed=true
      fi
{{ end }}
{{ end }}

      if [ "$verification_failed" = true ]; then
        send_user_state_event failed 1002 "network_configuration_failed"
        exit 1
      else
        send_user_state_event succeeded 1001 "network_configuration_success"
      fi
    
  - path: /var/lib/capi_network_settings/initial_configuration.sh
    permissions: '0755'
    content: |
      #!/bin/bash
      set -eu
      
      # Fetch metadata from Equinix Metal
      metadata=$(curl https://metadata.platformequinix.com/metadata)
      
      # Extract MAC addresses for eth0 and eth1
      mac_eth0=$(echo "$metadata" | jq -r '.network.interfaces[] | select(.name == "eth0") | .mac')
      mac_eth1=$(echo "$metadata" | jq -r '.network.interfaces[] | select(.name == "eth1") | .mac')
      
      # Check if MAC addresses were successfully extracted
      if [ -z "$mac_eth0" ] || [ -z "$mac_eth1" ]; then
        echo "Error: Failed to extract MAC addresses" >&2
        exit 1
      fi
      
      # Display extracted MAC addresses
      echo "Extracted MAC addresses - eth0: $mac_eth0, eth1: $mac_eth1"
      
      # Function to find network interface by MAC address
      find_interface_by_mac() {
        local mac="$1"
        for iface in $(ls /sys/class/net/); do
          iface_mac=$(ethtool -P "$iface" 2>/dev/null | awk '{print $NF}')
          if [ "$iface_mac" == "$mac" ]; then
            echo "$iface"
            return
          fi
        done
        echo "Interface not found for MAC $mac" >&2
        return 1
      }
      
      # Find interfaces for eth0 and eth1 MAC addresses
      iface_eth0=$(find_interface_by_mac "$mac_eth0")
      iface_eth1=$(find_interface_by_mac "$mac_eth1")
      
      # Check and replace eth0 in /var/lib/capi_network_settings/final_configuration.sh
      if grep -q "eth0" /var/lib/capi_network_settings/final_configuration.sh; then
        sed -i "s/eth0/${iface_eth0}/g" /var/lib/capi_network_settings/final_configuration.sh
        echo "Replaced eth0 with ${iface_eth0} in /var/lib/capi_network_settings/final_configuration.sh"
      else
        echo "No occurrences of eth0 found in /var/lib/capi_network_settings/final_configuration.sh. No changes made."
      fi
      
      # Check and replace eth1 in /var/lib/capi_network_settings/final_configuration.sh
      if grep -q "eth1" /var/lib/capi_network_settings/final_configuration.sh; then
        sed -i "s/eth1/${iface_eth1}/g" /var/lib/capi_network_settings/final_configuration.sh
        echo "Replaced eth1 with ${iface_eth1} in /var/lib/capi_network_settings/final_configuration.sh"
      else
        echo "No occurrences of eth1 found in /var/lib/capi_network_settings/final_configuration.sh. No changes made."
      fi

runcmd:
  - /var/lib/capi_network_settings/initial_configuration.sh
  - /var/lib/capi_network_settings/final_configuration.sh
`