package bridge

/*
import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const lanServiceName = "_kobashi._tcp.local."

var mdnsAddress = &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}

type lanServer struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}
type lanInterface struct {
	iface net.Interface
	ip    net.IP
}
type lanService struct {
	conns []*net.UDPConn
	wg    sync.WaitGroup
}
type dnsRecord struct {
	name  string
	kind  uint16
	start int
	end   int
	ttl   uint32
}

func lanInterfaces() ([]lanInterface, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var out []lanInterface
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagMulticast == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, err := iface.Addrs()
		if err != nil {
			return nil, err
		}
		for _, address := range addresses {
			ip, _, err := net.ParseCIDR(address.String())
			if err == nil && ip.To4() != nil && (ip.IsPrivate() || ip.IsLinkLocalUnicast()) {
				out = append(out, lanInterface{iface, ip.To4()})
			}
		}
	}
	return out, nil
}

func dnsName(name string) []byte {
	var out []byte
	for _, label := range strings.Split(strings.TrimSuffix(name, "."), ".") {
		out = append(out, byte(len(label)))
		out = append(out, label...)
	}
	return append(out, 0)
}

func readDNSName(packet []byte, offset int) (string, int, error) {
	labels := []string{}
	next := -1
	for steps := 0; steps < 128; steps++ {
		if offset < 0 || offset >= len(packet) {
			break
		}
		size := int(packet[offset])
		offset++
		if size == 0 {
			if next == -1 {
				next = offset
			}
			return strings.Join(labels, ".") + ".", next, nil
		}
		if size&0xc0 == 0xc0 {
			if offset >= len(packet) {
				break
			}
			if next == -1 {
				next = offset + 1
			}
			offset = (size&0x3f)<<8 | int(packet[offset])
		} else if size <= 63 && offset+size <= len(packet) {
			labels = append(labels, string(packet[offset:offset+size]))
			offset += size
		} else {
			break
		}
	}
	return "", 0, errors.New("invalid mDNS name")
}

func dnsRecords(packet []byte) ([]dnsRecord, bool, error) {
	if len(packet) < 12 {
		return nil, false, errors.New("short mDNS packet")
	}
	count := int(binary.BigEndian.Uint16(packet[4:6]))
	response := binary.BigEndian.Uint16(packet[2:4])&0x8000 != 0
	if response {
		count += int(binary.BigEndian.Uint16(packet[6:8])) + int(binary.BigEndian.Uint16(packet[8:10])) + int(binary.BigEndian.Uint16(packet[10:12]))
	}
	if count > 128 {
		return nil, response, errors.New("too many mDNS records")
	}
	offset := 12
	questions := int(binary.BigEndian.Uint16(packet[4:6]))
	var records []dnsRecord
	for i := 0; i < count; i++ {
		name, next, err := readDNSName(packet, offset)
		if err != nil || next+4 > len(packet) {
			return nil, response, errors.New("invalid mDNS record")
		}
		kind := binary.BigEndian.Uint16(packet[next : next+2])
		offset = next + 4
		record := dnsRecord{name: strings.ToLower(name), kind: kind}
		if i >= questions {
			if offset+6 > len(packet) {
				return nil, response, errors.New("short mDNS answer")
			}
			record.ttl = binary.BigEndian.Uint32(packet[offset : offset+4])
			size := int(binary.BigEndian.Uint16(packet[offset+4 : offset+6]))
			record.start, record.end = offset+6, offset+6+size
			offset = record.end
			if offset > len(packet) {
				return nil, response, errors.New("short mDNS data")
			}
		}
		records = append(records, record)
	}
	return records, response, nil
}

func lanAnnouncement(instance, target string, ip net.IP, port int, ttl uint32) []byte {
	packet := make([]byte, 12)
	binary.BigEndian.PutUint16(packet[2:4], 0x8400)
	binary.BigEndian.PutUint16(packet[6:8], 4)
	appendRecord := func(name string, kind uint16, data []byte) {
		packet = append(packet, dnsName(name)...)
		header := make([]byte, 10)
		binary.BigEndian.PutUint16(header[0:2], kind)
		class := uint16(0x8001)
		if kind == 12 {
			class = 1
		}
		binary.BigEndian.PutUint16(header[2:4], class)
		binary.BigEndian.PutUint32(header[4:8], ttl)
		binary.BigEndian.PutUint16(header[8:10], uint16(len(data)))
		packet = append(packet, header...)
		packet = append(packet, data...)
	}
	appendRecord(lanServiceName, 12, dnsName(instance))
	srv := make([]byte, 6)
	binary.BigEndian.PutUint16(srv[4:6], uint16(port))
	appendRecord(instance, 33, append(srv, dnsName(target)...))
	appendRecord(instance, 16, append([]byte{10}, []byte("protocol=1")...))
	appendRecord(target, 1, ip.To4())
	return packet
}

func advertiseLAN(listen string) (*lanService, error) {
	host, portText, err := net.SplitHostPort(listen)
	if err != nil {
		return nil, err
	}
	ip := net.ParseIP(host)
	if ip.IsLoopback() {
		return nil, nil
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return nil, err
	}
	interfaces, err := lanInterfaces()
	if err != nil {
		return nil, err
	}
	service := &lanService{}
	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}
	label := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' {
			return r
		}
		return '-'
	}, hostname)
	if len(label) > 40 {
		label = label[:40]
	}
	id := strings.ToLower(rand.Text()[:8])
	instance := label + "-" + id + "." + lanServiceName
	target := "kobashi-" + id + ".local."
	var failures error
	for _, iface := range interfaces {
		if !ip.IsUnspecified() && !ip.Equal(iface.ip) {
			continue
		}
		conn, err := net.ListenMulticastUDP("udp4", &iface.iface, mdnsAddress)
		if err == nil {
			err = configureMulticast(conn, iface.ip)
			if err != nil {
				conn.Close()
			}
		}
		if err != nil {
			failures = errors.Join(failures, err)
			continue
		}
		service.conns = append(service.conns, conn)
		packet := lanAnnouncement(instance, target, iface.ip, port, 120)
		service.wg.Add(1)
		go func() {
			defer service.wg.Done()
			buffer := make([]byte, 9000)
			for {
				n, source, err := conn.ReadFromUDP(buffer)
				if err != nil {
					if !errors.Is(err, net.ErrClosed) {
						log.Printf("Server mDNS listener: %v", err)
					}
					return
				}
				records, response, err := dnsRecords(buffer[:n])
				if err != nil || response {
					continue
				}
				for _, record := range records {
					if record.name == lanServiceName || record.name == strings.ToLower(instance) || record.name == target {
						// Browsing uses the QU bit and an ephemeral port; reply directly on the same interface.
						if _, err := conn.WriteToUDP(packet, source); err != nil {
							log.Printf("Server mDNS response: %v", err)
						}
						break
					}
				}
			}
		}()
	}
	if len(service.conns) == 0 {
		return nil, fmt.Errorf("no usable IPv4 multicast interface: %w", errors.Join(errors.New("manual Server URL remains available"), failures))
	}
	if failures != nil {
		log.Printf("Some Server discovery interfaces are unavailable: %v", failures)
	}
	return service, nil
}

func (s *lanService) Close() {
	for _, conn := range s.conns {
		if err := conn.Close(); err != nil {
			log.Printf("Server mDNS close: %v", err)
		}
	}
	s.wg.Wait()
}

func lanAnswers(packet []byte, source net.IP) []lanServer {
	records, response, err := dnsRecords(packet)
	if err != nil || !response || (!source.IsPrivate() && !source.IsLinkLocalUnicast() && !source.IsLoopback()) {
		return nil
	}
	instances := map[string]bool{}
	addresses := map[string]net.IP{}
	for _, record := range records {
		if record.name == lanServiceName && record.kind == 12 && record.ttl > 0 {
			name, end, err := readDNSName(packet, record.start)
			if err == nil && end <= record.end {
				instances[strings.ToLower(name)] = true
			}
		}
		if record.kind == 1 && record.ttl > 0 && record.end-record.start == 4 {
			ip := net.IP(packet[record.start:record.end])
			if ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLoopback() {
				addresses[record.name] = ip
			}
		}
	}
	var out []lanServer
	for _, record := range records {
		if record.kind != 33 || !instances[record.name] || record.ttl == 0 || record.end-record.start < 7 {
			continue
		}
		port := binary.BigEndian.Uint16(packet[record.start+4 : record.start+6])
		target, end, err := readDNSName(packet, record.start+6)
		ip := addresses[strings.ToLower(target)]
		if err == nil && end <= record.end && ip != nil && port != 0 {
			out = append(out, lanServer{strings.TrimSuffix(record.name, "."+lanServiceName),
				"http://" + net.JoinHostPort(ip.String(), strconv.Itoa(int(port)))})
		}
	}
	return out
}

func discoverLAN(ctx context.Context) (object, error) {
	interfaces, err := lanInterfaces()
	if err != nil {
		return nil, err
	}
	query := make([]byte, 12)
	binary.BigEndian.PutUint16(query[4:6], 1)
	query = append(query, dnsName(lanServiceName)...)
	query = append(query, 0, 12, 0x80, 1)
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	deadline, _ := ctx.Deadline()
	results := make(chan lanServer, 64)
	readErrors := make(chan error, len(interfaces))
	var wg sync.WaitGroup
	var failures error
	for _, iface := range interfaces {
		conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: iface.ip})
		if err != nil {
			failures = errors.Join(failures, err)
			continue
		}
		if err = conn.SetDeadline(deadline); err == nil {
			err = configureMulticast(conn, iface.ip)
		}
		if err == nil {
			_, err = conn.WriteToUDP(query, mdnsAddress)
		}
		if err != nil {
			conn.Close()
			failures = errors.Join(failures, err)
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer conn.Close()
			stop := context.AfterFunc(ctx, func() { conn.Close() })
			defer stop()
			packet := make([]byte, 9000)
			for {
				n, source, err := conn.ReadFromUDP(packet)
				if err != nil {
					var timeout net.Error
					if ctx.Err() == nil && !errors.Is(err, net.ErrClosed) && !(errors.As(err, &timeout) && timeout.Timeout()) {
						readErrors <- err
					}
					return
				}
				for _, server := range lanAnswers(packet[:n], source.IP) {
					select {
					case results <- server:
					default:
					}
				}
			}
		}()
	}
	go func() { wg.Wait(); close(readErrors); close(results) }()
	out := []lanServer{}
	seen := map[string]bool{}
	for server := range results {
		if !seen[server.URL] {
			seen[server.URL] = true
			out = append(out, server)
		}
	}
	for err := range readErrors {
		failures = errors.Join(failures, err)
	}
	if len(interfaces) == 0 || (len(out) == 0 && failures != nil) {
		return nil, fmt.Errorf("LAN discovery unavailable; enter a Server URL manually: %w", errors.Join(errors.New("no reachable IPv4 multicast interface"), failures))
	}
	if failures != nil {
		log.Printf("Some Client discovery interfaces are unavailable: %v", failures)
	}
	if ctx.Err() != nil && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil, ctx.Err()
	}
	return object{"servers": out}, nil
}
*/
