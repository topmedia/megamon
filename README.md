megamon
=======

Collector tool for LSI MegaRAID-equipped servers reporting RAID status back to
an Elasticsearch instance.

        Usage of megamon:
          -cli string
                Location of the MegaCli binary (default "/opt/MegaRAID/MegaCli/MegaCli64")
          -destination string
                Output destination (default "localhost:9200")
          -index string
                Elasticsearch index to write to (default "euronas")
          -interval string
                Reporting interval (default "5m")
          -shipper string
                Value of the shipper field to use (default "euronas")
