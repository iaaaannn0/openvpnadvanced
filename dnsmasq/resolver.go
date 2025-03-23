package dnsmasq

import (
	"bufio"
	"log"
	"openvpnadvanced/doh"
	"os"
	"strings"
)

type Rule struct {
	Suffix string
}

// MatchesRules checks if a domain matches any of the rules
func MatchesRules(domain string, rules []Rule) bool {
	for _, rule := range rules {
		if strings.HasSuffix(domain, rule.Suffix) {
			return true
		}
	}
	return false
}

// ResolveRecursive performs a full resolution: A, AAAA, CNAME fallback
func ResolveRecursive(domain string, rules []Rule, cache *Cache) (bool, string) {
	visited := make(map[string]bool)
	current := domain

	for depth := 0; depth < 10; depth++ {
		if visited[current] {
			log.Printf("⚠️ Circular CNAME detected for %s", domain)
			return false, ""
		}
		visited[current] = true

		// Check cache
		if cachedIP, ok := cache.Get(current); ok {
			log.Printf("[CACHE] %s ➜ %s", current, cachedIP)
			return MatchesRules(current, rules), cachedIP
		}

		// Try A or fallback to CNAME
		ip, cname, err := doh.QueryWithCNAME(current)
		if err == nil && ip != "" {
			log.Printf("[A] %s ➜ %s", current, ip)
			cache.Set(current, ip)
			return MatchesRules(current, rules), ip
		}

		// Try AAAA (IPv6)
		ipv6, err := doh.QueryAAAA(current)
		if err == nil && ipv6 != "" {
			log.Printf("[AAAA] %s ➜ %s", current, ipv6)
			cache.Set(current, ipv6)
			return MatchesRules(current, rules), ipv6
		}

		// Follow CNAME if present
		if cname != "" {
			log.Printf("[CNAME] %s ➜ %s", current, cname)
			current = cname
			continue
		}

		// Try all types as last resort
		allRecords, err := doh.QueryAll(current)
		if err == nil && len(allRecords) > 0 {
			for _, recordList := range allRecords {
				for _, data := range recordList {
					log.Printf("[DNS] %s ➜ %s", current, data)
					cache.Set(current, data)
					return MatchesRules(current, rules), data
				}
			}
		}

		break
	}

	log.Printf("❌ Resolution failed for %s", domain)
	return false, ""
}

// LoadDomainRules loads DOMAIN-SUFFIX rules from a file
func LoadDomainRules(path string) ([]Rule, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var rules []Rule
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "DOMAIN-SUFFIX,") {
			suffix := strings.TrimPrefix(line, "DOMAIN-SUFFIX,")
			rules = append(rules, Rule{Suffix: suffix})
		}
	}

	return rules, nil

}

// ResolveWithCNAME exposes recursive resolution and returns CNAME (if any)
func ResolveWithCNAME(domain string, rules []Rule, cache *Cache) (bool, string, string) {
	visited := make(map[string]bool)
	current := domain

	for depth := 0; depth < 10; depth++ {
		if visited[current] {
			log.Printf("⚠️ Circular CNAME detected for %s", domain)
			return false, "", ""
		}
		visited[current] = true

		if cachedIP, ok := cache.Get(current); ok {
			return MatchesRules(current, rules), cachedIP, ""
		}

		ip, cname, err := doh.QueryWithCNAME(current)
		if err == nil && ip != "" {
			cache.Set(current, ip)
			return MatchesRules(current, rules), ip, cname
		}

		ipv6, err := doh.QueryAAAA(current)
		if err == nil && ipv6 != "" {
			cache.Set(current, ipv6)
			return MatchesRules(current, rules), ipv6, ""
		}

		if cname != "" {
			current = cname
			continue
		}

		allRecords, err := doh.QueryAll(current)
		if err == nil && len(allRecords) > 0 {
			for _, recordList := range allRecords {
				for _, data := range recordList {
					cache.Set(current, data)
					return MatchesRules(current, rules), data, ""
				}
			}
		}

		break
	}

	return false, "", ""
}
