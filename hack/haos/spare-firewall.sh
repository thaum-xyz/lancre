# Spare Pi host firewall (ssh -p 22222 root@<pi> 'sh -s' < spare-firewall.sh). Not persistent: re-apply after
# every boot before Core runs the restored config. Drops new connections from the host
# (Core is host-networked) and apps (DOCKER-USER) to 192.168.0.0/16 except the Pi's own
# subnet, and to the tailnet; replies to this Mac pass via conntrack.
set -e
OWN=$(ip -4 -o addr show end0 | awk "{print \$4}" | head -1)
for c in OUTPUT DOCKER-USER; do
  iptables -N LANCRE-BLOCK 2>/dev/null || iptables -F LANCRE-BLOCK
  iptables -C $c -j LANCRE-BLOCK 2>/dev/null || iptables -I $c 1 -j LANCRE-BLOCK
done
iptables -A LANCRE-BLOCK -m conntrack --ctstate ESTABLISHED,RELATED -j RETURN
iptables -A LANCRE-BLOCK -d "$OWN" -j RETURN
for n in 192.168.0.0/16 100.64.0.0/10; do iptables -A LANCRE-BLOCK -d $n -j DROP; done
ip6tables -N LANCRE-BLOCK 2>/dev/null || ip6tables -F LANCRE-BLOCK
for c in OUTPUT FORWARD; do ip6tables -C $c -j LANCRE-BLOCK 2>/dev/null || ip6tables -I $c 1 -j LANCRE-BLOCK; done
ip6tables -A LANCRE-BLOCK -m conntrack --ctstate ESTABLISHED,RELATED -j RETURN
ip6tables -A LANCRE-BLOCK -d fd7a:115c:a1e0::/48 -j DROP
echo "firewall: $(iptables -S LANCRE-BLOCK | grep -c DROP) v4 + $(ip6tables -S LANCRE-BLOCK | grep -c DROP) v6 rules"
