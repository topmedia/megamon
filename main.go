package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/url"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/belogik/goes"
)

var (
	interval       = flag.String("interval", "5m", "Reporting interval")
	index          = flag.String("index", "euronas", "Elasticsearch index to write to")
	shipper        = flag.String("shipper", "euronas", "Value of the shipper field to use")
	destination    = flag.String("destination", "localhost:9200", "Output destination")
	cli_path       = flag.String("cli", "/opt/MegaRAID/MegaCli/MegaCli64", "Location of the MegaCli binary")
	line_matcher   = regexp.MustCompile(`^(Slot Number|Inquiry Data|Media Error Count|Other Error Count|Firmware state|Drive has flagged a S.M.A.R.T alert ):`)
	serial_matcher = regexp.MustCompile(`(\w+)(ST\d000[\w-]+)`)
)

type SlotStatus struct {
	Number          int
	MediaErrorCount int
	OtherErrorCount int
	SerialNumber    string
	ModelNumber     string
	FirmwareVersion string
	SmartAlert      bool
	State           string
}

func (s *SlotStatus) SplitInquiryData(inquiry string) {
	data := strings.Split(inquiry, ":")[1]
	fields := strings.Fields(data)

	if len(fields) < 3 {
		if sm := serial_matcher.FindAllStringSubmatch(fields[0], -1); sm != nil {
			s.SerialNumber = sm[0][1]
		}
	} else {
		s.SerialNumber = fields[0]
	}

	s.FirmwareVersion = fields[len(fields)-1]
	s.ModelNumber = strings.TrimSpace(
		strings.Replace(strings.Replace(data, s.SerialNumber, "", -1),
			s.FirmwareVersion, "", -1))
}

func (s *SlotStatus) Document() goes.Document {
	y, m, d := time.Now().Date()
	return goes.Document{
		Index: fmt.Sprintf("%s-%d.%02d.%02d", *index, y, m, d),
		Type:  "megamon",
		Fields: map[string]interface{}{
			"@timestamp":        time.Now(),
			"shipper":           *shipper,
			"slot_number":       s.Number,
			"media_error_count": s.MediaErrorCount,
			"other_error_count": s.OtherErrorCount,
			"serial_number":     s.SerialNumber,
			"model_number":      s.ModelNumber,
			"firmware_version":  s.FirmwareVersion,
			"smart_alert":       s.SmartAlert,
			"state":             s.State,
		},
	}
}

func SplitFieldValue(s string) (string, string) {
	splits := strings.Split(s, ":")
	return strings.TrimSpace(splits[0]), strings.TrimSpace(splits[1])
}

func FormatNumber(value string) int {
	number, err := strconv.Atoi(value)

	if err != nil {
		log.Fatalf("Parsing Slot Number failed: %v", err)
	}

	return number
}

func main() {
	flag.Parse()

	host, port, err := net.SplitHostPort(*destination)

	if err != nil {
		log.Fatalf("Error parsing destination: %v", err)
	}

	es := goes.NewConnection(host, port)

	d, err := time.ParseDuration(*interval)

	if err != nil {
		log.Fatalf("Error parsing interval: %v", err)
	}

	for {
		out, err := exec.Command(*cli_path, "-PDList", "-a0").Output()
		if err != nil {
			log.Fatalf("Running LSI command failed: %v", err)
		}

		var (
			slots []SlotStatus
			slot  SlotStatus
		)

		for _, l := range regexp.MustCompile("\r?\n").Split(string(out), -1) {
			if !line_matcher.MatchString(l) {
				continue
			}

			field, value := SplitFieldValue(l)

			switch field {
			case "Slot Number":
				number := FormatNumber(value)

				if number > 0 {
					slots = append(slots, slot)
				}
				slot = SlotStatus{Number: number}
			case "Inquiry Data":
				slot.SplitInquiryData(l)
			case "Media Error Count":
				number := FormatNumber(value)
				slot.MediaErrorCount = number
			case "Other Error Count":
				number := FormatNumber(value)
				slot.OtherErrorCount = number
			case "Firmware state":
				slot.State = value
			case "Drive has flagged a S.M.A.R.T alert":
				if value == "No" {
					slot.SmartAlert = false
				} else {
					slot.SmartAlert = true
				}

			}
		}

		// Append last slot
		slots = append(slots, slot)

		for _, s := range slots {
			_, err = es.Index(s.Document(), url.Values{})

			if err != nil {
				log.Fatalf("Error indexing results: %s", err)
			}
		}

		log.Printf("Done indexing. Sleeping for %s", *interval)
		time.Sleep(d)
	}
}
