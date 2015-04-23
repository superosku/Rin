package rin

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"strings"

	"github.com/lib/pq"
	"gopkg.in/yaml.v1"
)

const (
	S3URITemplate       = "s3://%s/%s"
	CredentialsTemplate = "aws_access_key_id=%s;aws_secret_access_key=%s"
	SQLTemplate         = "/* Rin */ COPY %s FROM %s CREDENTIALS '%s' REGION '%s' %s"
	// Prefix SQL comment "/* Rin */". Because a query which start with "COPY", pq expect a PostgreSQL COPY command response, but a Redshift response is different it.
)

func quoteValue(v string) string {
	return "'" + strings.Replace(v, "'", "''", -1) + "'"
}

type Config struct {
	QueueName   string      `yaml:"queue_name"`
	Targets     []*Target   `yaml:"targets"`
	Credentials Credentials `yaml:"credentials"`
	Redshift    *Redshift   `yaml:"redshift"`
	S3          *S3         `yaml:"s3"`
	SQLOption   string      `yaml:"sql_option"`
}

type Credentials struct {
	AWS_ACCESS_KEY_ID     string `yaml:"aws_access_key_id"`
	AWS_SECRET_ACCESS_KEY string `yaml:"aws_secret_access_key"`
	AWS_REGION            string `yaml:"aws_region"`
}

type Target struct {
	Redshift  *Redshift `yaml:"redshift"`
	S3        *S3       `yaml:"s3"`
	SQLOption string    `yaml:"sql_option"`
}

type SQLParam struct {
	Table  string
	Option string
}

func (t *Target) Match(bucket, key string) bool {
	return bucket == t.S3.Bucket && strings.HasPrefix(key, t.S3.KeyPrefix)
}

func (t *Target) MatchEventRecord(r EventRecord) bool {
	return r.S3.Bucket.Name == t.S3.Bucket && strings.HasPrefix(r.S3.Object.Key, t.S3.KeyPrefix)
}

func (t *Target) BuildCopySQL(key string, cred Credentials) (string, error) {
	var table string
	if t.Redshift.Schema == "" {
		table = pq.QuoteIdentifier(t.Redshift.Table)
	} else {
		table = pq.QuoteIdentifier(t.Redshift.Schema) + "." + pq.QuoteIdentifier(t.Redshift.Table)
	}
	query := fmt.Sprintf(
		SQLTemplate,
		table,
		quoteValue(fmt.Sprintf(S3URITemplate, t.S3.Bucket, key)),
		fmt.Sprintf(CredentialsTemplate, cred.AWS_ACCESS_KEY_ID, cred.AWS_SECRET_ACCESS_KEY),
		t.S3.Region,
		t.SQLOption,
	)
	return query, nil
}

type S3 struct {
	Region    string `yaml:"region"`
	Bucket    string `yaml:"bucket"`
	KeyPrefix string `yaml:"key_prefix"`
}

func (s3 S3) String() string {
	return fmt.Sprintf(S3URITemplate, s3.Bucket, s3.KeyPrefix+"*")
}

type Redshift struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	DBName   string `yaml:"dbname"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Schema   string `yaml:"schema"`
	Table    string `yaml:"table"`
}

func (r Redshift) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s",
		url.QueryEscape(r.User), url.QueryEscape(r.Password),
		url.QueryEscape(r.Host), r.Port, url.QueryEscape(r.DBName),
	)
}

func (r Redshift) String() string {
	return r.DSN() + "?table=" + r.Table
}

func LoadConfig(path string) (*Config, error) {
	src, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	err = yaml.Unmarshal(src, &c)
	if err != nil {
		return nil, err
	}
	(&c).merge()
	return &c, (&c).validate()
}

func (c *Config) validate() error {
	if c.QueueName == "" {
		return fmt.Errorf("queue_name required")
	}
	if len(c.Targets) == 0 {
		return fmt.Errorf("no targets defined")
	}
	return nil
}

func (c *Config) merge() {
	cr := c.Redshift
	cs := c.S3
	for _, t := range c.Targets {
		if t.SQLOption == "" {
			t.SQLOption = c.SQLOption
		}
		tr := t.Redshift
		if tr.Host == "" {
			tr.Host = cr.Host
		}
		if tr.Port == 0 {
			tr.Port = cr.Port
		}
		if tr.DBName == "" {
			tr.DBName = cr.DBName
		}
		if tr.User == "" {
			tr.User = cr.User
		}
		if tr.Password == "" {
			tr.Password = cr.Password
		}
		if tr.Schema == "" {
			tr.Schema = cr.Schema
		}
		if tr.Table == "" {
			tr.Table = cr.Table
		}

		ts := t.S3
		if ts.Bucket == "" {
			ts.Bucket = cs.Bucket
		}
		if ts.Region == "" {
			ts.Region = cs.Region
		}
		if ts.KeyPrefix == "" {
			ts.KeyPrefix = cs.KeyPrefix
		}
	}
}
