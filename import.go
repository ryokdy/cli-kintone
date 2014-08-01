package main

import (
	"github.com/ryokdy/go-kintone"
	"github.com/djimenez/iconv-go"
	"fmt"
	"strings"
	"time"
	"encoding/csv"
	"io"
	"os"
	"strconv"
	"regexp"
)

func getReader(file *os.File) io.Reader {
	var reader io.Reader
	if encoding != "utf-8" {
		reader, _ = iconv.NewReader(file, encoding, "utf-8")
	} else {
		reader = file
	}
	return reader
}

func readCsv(app *kintone.App, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := csv.NewReader(getReader(file))

	head := true
	updating := false
	records := make([]*kintone.Record, 0, ROW_LIMIT)
	var fieldTypes []string
	
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}
		//log.Printf("%#v", record)
		if head && fields == nil {
			fields =make([]string, len(row))
			fieldTypes = make([]string, len(row))
			for i, col := range row {
				re := regexp.MustCompile("^(.*)\\[(.*)\\]$")
				match := re.FindStringSubmatch(col)
				if match != nil {
					fields[i] = match[1]
					fieldTypes[i] = match[2]
					col = fields[i]
				}
				if col == "$id" {
					updating = true
				}
			}
			head = false
		} else {
			var id uint64 = 0
			var err error
			record := make(map[string]interface{})
			
			for i, col := range row {
				fieldName := fields[i]
				if fieldName == "$id" {
					id, err = strconv.ParseUint(col, 10, 64)
					if err != nil {
						return fmt.Errorf("Invalid record ID: %v", col)
					}
				} else {
					field := getField(fieldTypes[i], col, updating)
					if field != nil {
						record[fieldName] = field
					}
				}
			}
			if updating {
				records = append(records, kintone.NewRecordWithId(id, record))
			} else {
				records = append(records, kintone.NewRecord(record))
			}
			if len(records) >= ROW_LIMIT {
				upsert(app, records[:], updating)
				records = make([]*kintone.Record, 0, ROW_LIMIT)
			}
		}
	}
	if len(records) > 0 {
		return upsert(app, records[:], updating)
	}

	return nil
}

func upsert(app *kintone.App, recs []*kintone.Record, updating bool)  error {
	var err error
	if updating {
		err = app.UpdateRecords(recs, true)
	} else {
		if deleteAll {
			deleteRecords(app)
			deleteAll = false
		}
		_, err = app.AddRecords(recs)
	}

	return err
}

func deleteRecords(app *kintone.App) error {
	offset := int64(0)
	for ;;offset += ROW_LIMIT {
		ids := make([]uint64, 0, ROW_LIMIT)
		records, err := getRecords(app, []string{"$id"}, offset)
		if err != nil {
			return err
		}
		for _, record := range records {
			id := record.Id()
			ids = append(ids, id)
		}

		err = app.DeleteRecords(ids)
		if err != nil {
			return err
		}
		
		if len(records) < ROW_LIMIT {
			break
		}
	}

	return nil
}

func getField(fieldType string, value string, updating bool) interface{} {
	switch fieldType {
	case kintone.FT_SINGLE_LINE_TEXT:
		return kintone.SingleLineTextField(value)
	case kintone.FT_MULTI_LINE_TEXT:
		return kintone.MultiLineTextField(value)
	case kintone.FT_RICH_TEXT:
		return kintone.RichTextField(value)
	case kintone.FT_DECIMAL:
		return kintone.DecimalField(value)
	case kintone.FT_CALC:
		return nil
	case kintone.FT_CHECK_BOX:
		if len(value) == 0 {
			return kintone.CheckBoxField([]string{})
		} else {
			return kintone.CheckBoxField(strings.Split(value, "\n"))
		}
	case kintone.FT_RADIO:
		return kintone.RadioButtonField(value)
	case kintone.FT_SINGLE_SELECT:
		if len(value) == 0 {
			return kintone.SingleSelectField{Valid: false}
		} else {
			return kintone.SingleSelectField{value, true}
		}
	case kintone.FT_MULTI_SELECT:
		if len(value) == 0 {
			return kintone.MultiSelectField([]string{})
		} else {
			return kintone.MultiSelectField(strings.Split(value, "\n"))
		}
	case kintone.FT_FILE:
		return nil
	case kintone.FT_LINK:
		return kintone.LinkField(value)
	case kintone.FT_DATE:
		if value == "" {
			return kintone.DateField{Valid: false}
		} else {
			dt, err := time.Parse("2006-01-02", value)
			if err == nil {
				return kintone.DateField{dt, true}
			}
		}
	case kintone.FT_TIME:
		if value == "" {
			return kintone.TimeField{Valid: false}
		} else {
			dt, err := time.Parse("15:04:05", value)
			if err == nil {
				return kintone.TimeField{dt, true}
			}
		}
	case kintone.FT_DATETIME:
		if value == "" {
			return kintone.DateTimeField{Valid: false}
		} else {
			dt, err := time.Parse(time.RFC3339, value)
			if err == nil {
				return kintone.DateTimeField{dt, true}
			}
		}
	case kintone.FT_USER:
		users := strings.Split(value, "\n")
		var ret kintone.UserField = []kintone.User{}
		for _, user := range users {
			if len(strings.TrimSpace(user)) > 0 {
				ret = append(ret, kintone.User{Code: user})
			}
		}
		return ret
	case kintone.FT_CATEGORY:
		return nil
	case kintone.FT_STATUS:
		return nil
	case kintone.FT_RECNUM:
		return nil
	case kintone.FT_ASSIGNEE:
		return nil
	case kintone.FT_CREATOR:
		if updating {
			return nil
		} else {
			return kintone.CreatorField{Code: value}
		}
	case kintone.FT_MODIFIER:
		if updating {
			return nil
		} else {
			return kintone.ModifierField{Code: value}
		}
	case kintone.FT_CTIME:
		if updating {
			return nil
		} else {
			dt, err := time.Parse(time.RFC3339, value)
			if err == nil {
				return kintone.CreationTimeField(dt)
			}
		}
	case kintone.FT_MTIME:
		if updating {
			return nil
		} else {
			dt, err := time.Parse(time.RFC3339, value)
			if err == nil {
				return kintone.ModificationTimeField(dt)
			}
		}
	case kintone.FT_SUBTABLE:
		return nil
	}

	return kintone.SingleLineTextField(value)

}
