/*
 * Minimalist Object Storage, (C) 2015 Minio, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package donut

import (
	"errors"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/minio-io/mc/pkg/client"
	"github.com/minio-io/mc/pkg/storage/donut"
)

// donutDriver - creates a new single disk drivers driver using donut
type donutDriver struct {
	donut donut.Donut
}

const (
	blockSize = 10 * 1024 * 1024
)

// IsValidBucketName reports whether bucket is a valid bucket name, per Amazon's naming restrictions.
// See http://docs.aws.amazon.com/AmazonS3/latest/dev/BucketRestrictions.html
func IsValidBucketName(bucket string) bool {
	if len(bucket) < 3 || len(bucket) > 63 {
		return false
	}
	if bucket[0] == '.' || bucket[len(bucket)-1] == '.' {
		return false
	}
	if match, _ := regexp.MatchString("\\.\\.", bucket); match == true {
		return false
	}
	// We don't support buckets with '.' in them
	match, _ := regexp.MatchString("^[a-zA-Z][a-zA-Z0-9\\-]+[a-zA-Z0-9]$", bucket)
	return match
}

// GetNewClient returns an initialized donut driver
func GetNewClient(path string) client.Client {
	d := new(donutDriver)
	d.donut, _ = donut.NewDonut(path)
	return d
}

// ListBuckets returns a list of buckets
func (d *donutDriver) ListBuckets() (results []*client.Bucket, err error) {
	buckets, err := d.donut.ListBuckets()
	if err != nil {
		return nil, err
	}
	for _, bucket := range buckets {
		t := client.XMLTime{
			Time: time.Now(),
		}
		result := &client.Bucket{
			Name: bucket,
			// TODO Add real created date
			CreationDate: t,
		}
		results = append(results, result)
	}
	return results, nil
}

// PutBucket creates a new bucket
func (d *donutDriver) PutBucket(bucketName string) error {
	if IsValidBucketName(bucketName) && !strings.Contains(bucketName, ".") {
		return d.donut.CreateBucket(bucketName)
	}
	return errors.New("Invalid bucket")
}

// Get retrieves an object and writes it to a writer
func (d *donutDriver) Get(bucketName, objectKey string) (body io.ReadCloser, size int64, err error) {
	reader, err := d.donut.GetObjectReader(bucketName, objectKey)
	if err != nil {
		return nil, 0, err
	}
	metadata, err := d.donut.GetObjectMetadata(bucketName, objectKey)
	if err != nil {
		return nil, 0, err
	}
	size, err = strconv.ParseInt(metadata["sys.size"], 10, 64)
	if err != nil {
		return nil, 0, err
	}
	return reader, size, nil
}

// GetPartial retrieves an object range and writes it to a writer
func (d *donutDriver) GetPartial(bucket, object string, start, length int64) (body io.ReadCloser, size int64, err error) {
	return nil, 0, errors.New("Not Implemented")
}

// Stat - gets metadata information about the object
func (d *donutDriver) Stat(bucket, object string) (size int64, date time.Time, err error) {
	return 0, time.Time{}, errors.New("Not Implemented")
}

// ListObjects - returns list of objects
func (d *donutDriver) ListObjects(bucketName, startAt, prefix, delimiter string, maxKeys int) (items []*client.Item, prefixes []*client.Prefix, err error) {
	objects, err := d.donut.ListObjects(bucketName)
	if err != nil {
		return nil, nil, err
	}
	sort.Strings(objects)
	if prefix != "" {
		objects = filterPrefix(objects, prefix)
		objects = removePrefix(objects, prefix)
	}
	if maxKeys <= 0 || maxKeys > 1000 {
		maxKeys = 1000
	}
	var actualObjects []string
	var commonPrefixes []string
	if strings.TrimSpace(delimiter) != "" {
		actualObjects = filterDelimited(objects, delimiter)
		commonPrefixes = filterNotDelimited(objects, delimiter)
		commonPrefixes = extractDir(commonPrefixes, delimiter)
		commonPrefixes = uniqueObjects(commonPrefixes)
	} else {
		actualObjects = objects
	}

	for _, prefix := range commonPrefixes {
		prefixes = append(prefixes, &client.Prefix{Prefix: prefix})
	}
	for _, object := range actualObjects {
		metadata, err := d.donut.GetObjectMetadata(bucketName, object)
		if err != nil {
			return nil, nil, err
		}
		t1, err := time.Parse(time.RFC3339Nano, metadata["sys.created"])
		if err != nil {
			return nil, nil, err
		}
		t := client.XMLTime{
			Time: t1,
		}
		size, err := strconv.ParseInt(metadata["sys.size"], 10, 64)
		if err != nil {
			return nil, nil, err
		}
		item := &client.Item{
			Key:          object,
			LastModified: t,
			Size:         size,
		}
		items = append(items, item)
	}
	return items, prefixes, nil
}

// Put creates a new object
func (d *donutDriver) Put(bucketName, objectKey string, size int64, contents io.Reader) error {
	writer, err := d.donut.GetObjectWriter(bucketName, objectKey)
	if err != nil {
		return err
	}
	if _, err := io.Copy(writer, contents); err != nil {
		return err
	}
	metadata := make(map[string]string)
	metadata["bucket"] = bucketName
	metadata["object"] = objectKey
	metadata["contentType"] = "application/octet-stream"
	if err = writer.SetMetadata(metadata); err != nil {
		return err
	}
	return writer.Close()
}