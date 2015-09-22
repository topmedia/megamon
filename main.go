package main

import (
	"flag"
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
	interval    = flag.String("interval", "5m", "Reporting interval")
	index       = flag.String("index", "euronas", "Elasticsearch index to write to")
	es_type     = flag.String("type", "euronas", "Elasticsearch type to use")
	destination = flag.String("destination", "localhost:9200", "Output destination")
	cli_path    = flag.String("cli", "/opt/MegaRAID/MegaCli/MegaCli64", "Location of the MegaCli binary")
	matcher     = regexp.MustCompile(`^(Slot Number|Inquiry Data|Media Error Count|Other Error Count|Firmware state|Drive has flagged a S.M.A.R.T alert ):`)
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
	fields := strings.Fields(strings.Split(inquiry, ":")[1])
	s.SerialNumber = fields[0]
	s.ModelNumber = fields[1]
	s.FirmwareVersion = fields[2]
}

func (s *SlotStatus) Document() goes.Document {
	return goes.Document{
		Index: *index,
		Type:  *es_type,
		Fields: map[string]interface{}{
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
			if !matcher.Match([]byte(l)) {
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
					slot.SmartAlert = true
				} else {
					slot.SmartAlert = false
				}

			}
		}

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