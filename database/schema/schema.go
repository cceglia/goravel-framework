package schema

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"sync"

	"github.com/goravel/framework/contracts/config"
	"github.com/goravel/framework/contracts/database/driver"
	contractsorm "github.com/goravel/framework/contracts/database/orm"
	contractsschema "github.com/goravel/framework/contracts/database/schema"
	"github.com/goravel/framework/contracts/log"
	"github.com/goravel/framework/errors"
	"github.com/goravel/framework/support/color"
)

var _ contractsschema.Schema = (*Schema)(nil)

// Option customizes a Schema built by NewSchema.
type Option func(*Schema)

// WithOrmResolver makes the schema resolve its Orm on every call while it is on the connection
// it was built for. A nil function means no resolver. The option has no effect on a schema
// built with a nil Orm or a nil driver.
func WithOrmResolver(resolve func() contractsorm.Orm) Option {
	return func(r *Schema) {
		r.resolveOrm = resolve
	}
}

// Schema is safe for concurrent schema operations. Switching the connection of a schema with
// SetConnection requires exclusive use: no other goroutine may use the schema during a switch.
// The lock only prevents data races on the connection fields and does not make a multi-call
// operation atomic. Concurrent migrations on one schema are not supported.
type Schema struct {
	config config.Config
	log    log.Log

	// mu guards every field that a connection switch writes: connection, orm, driver,
	// grammar, processor, prefix and schema. A call copies them under the read lock and
	// releases it before it runs queries, calls the resolver or calls another schema method.
	mu         sync.RWMutex
	connection string
	driver     driver.Driver
	grammar    driver.Grammar
	orm        contractsorm.Orm
	prefix     string
	processor  driver.Processor
	schema     string

	// buildConnection, buildState and resolveOrm are set once when the schema is built.
	// buildState keeps the driver values of the build connection and has no Orm.
	buildConnection string
	buildState      connectionState
	resolveOrm      func() contractsorm.Orm

	migrations       []contractsschema.Migration
	goTypes          []contractsschema.GoType
	models           []any
	modelsByFullName map[string]any
}

// connectionState is a copy of the fields that a connection switch writes.
type connectionState struct {
	orm       contractsorm.Orm
	driver    driver.Driver
	grammar   driver.Grammar
	processor driver.Processor
	prefix    string
	schema    string
}

func NewSchema(config config.Config, log log.Log, orm contractsorm.Orm, driver driver.Driver, migrations []contractsschema.Migration, options ...Option) (*Schema, error) {
	if driver == nil {
		return &Schema{
			config:           config,
			driver:           driver,
			log:              log,
			migrations:       migrations,
			orm:              orm,
			goTypes:          defaultGoTypes(),
			models:           make([]any, 0),
			modelsByFullName: make(map[string]any),
		}, nil
	}

	writers := driver.Pool().Writers
	if len(writers) == 0 {
		return nil, errors.DatabaseConfigNotFound
	}

	prefix := writers[0].Prefix
	schema := writers[0].Schema
	grammar := driver.Grammar()
	processor := driver.Processor()

	built := &Schema{
		config:           config,
		driver:           driver,
		grammar:          grammar,
		log:              log,
		migrations:       migrations,
		orm:              orm,
		prefix:           prefix,
		processor:        processor,
		schema:           schema,
		goTypes:          defaultGoTypes(),
		models:           make([]any, 0),
		modelsByFullName: make(map[string]any),
	}

	if orm != nil {
		for _, option := range options {
			option(built)
		}

		if built.resolveOrm != nil {
			built.buildConnection = orm.Name()
			built.connection = built.buildConnection
			built.buildState = connectionState{
				driver:    driver,
				grammar:   grammar,
				processor: processor,
				prefix:    prefix,
				schema:    schema,
			}
		}
	}

	return built, nil
}

// state copies the connection fields under the read lock. In resolver mode, which means a
// resolver is set and the schema is on its build connection, it resolves the Orm after the
// lock is released.
func (r *Schema) state() connectionState {
	r.mu.RLock()
	state := connectionState{
		orm:       r.orm,
		driver:    r.driver,
		grammar:   r.grammar,
		processor: r.processor,
		prefix:    r.prefix,
		schema:    r.schema,
	}
	resolve := r.resolveOrm
	resolverMode := resolve != nil && r.connection == r.buildConnection
	r.mu.RUnlock()

	if resolverMode {
		state.orm = resolve()
	}

	return state
}

func (r *Schema) Connection(name string) contractsschema.Schema {
	schema, err := r.newConnection(name)
	if err != nil {
		r.log.Panic(errors.SchemaConnectionNotFound.Args(name).SetModule(errors.ModuleSchedule).Error())
		return nil
	}

	return schema
}

func (r *Schema) Create(table string, callback func(table contractsschema.Blueprint)) error {
	blueprint := r.createBlueprint(table)
	blueprint.Create()
	callback(blueprint)

	if err := r.build(blueprint); err != nil {
		return errors.SchemaFailedToCreateTable.Args(table, err)
	}

	return nil
}

func (r *Schema) Drop(table string) error {
	blueprint := r.createBlueprint(table)
	blueprint.Drop()

	if err := r.build(blueprint); err != nil {
		return errors.SchemaFailedToDropTable.Args(table, err)
	}

	return nil
}

func (r *Schema) DropAllTables() error {
	state := r.state()
	tables, err := r.tables(state)
	if err != nil {
		return err
	}

	sqls := state.grammar.CompileDropAllTables(state.schema, tables)
	if sqls == nil {
		return nil
	}

	return state.orm.Transaction(func(tx contractsorm.Query) error {
		for _, sql := range sqls {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}

		return nil
	})
}

func (r *Schema) DropAllTypes() error {
	state := r.state()
	types, err := r.types(state)
	if err != nil {
		return err
	}

	return state.orm.Transaction(func(tx contractsorm.Query) error {
		for _, sql := range state.grammar.CompileDropAllTypes(state.schema, types) {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}

		return nil
	})
}

func (r *Schema) DropAllViews() error {
	state := r.state()
	views, err := r.views(state)
	if err != nil {
		return err
	}

	sqls := state.grammar.CompileDropAllViews(state.schema, views)
	if sqls == nil {
		return nil
	}

	return state.orm.Transaction(func(tx contractsorm.Query) error {
		for _, sql := range sqls {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}

		return nil
	})
}

func (r *Schema) DropColumns(table string, columns []string) error {
	blueprint := r.createBlueprint(table)
	blueprint.DropColumn(columns...)

	if err := r.build(blueprint); err != nil {
		return errors.SchemaFailedToDropColumns.Args(table, err)
	}

	return nil
}

func (r *Schema) DropIfExists(table string) error {
	blueprint := r.createBlueprint(table)
	blueprint.DropIfExists()

	if err := r.build(blueprint); err != nil {
		return errors.SchemaFailedToDropTable.Args(table, err)
	}

	return nil
}

func (r *Schema) Extend(extend contractsschema.Extension) contractsschema.Schema {
	r.extendGoTypes(extend.GoTypes)
	r.extendModels(extend.Models)
	return r
}

func (r *Schema) GetColumnListing(table string) []string {
	columns, err := r.GetColumns(table)
	if err != nil {
		r.log.Errorf("failed to get %s columns: %v", table, err)
		return nil
	}

	var names []string
	for _, column := range columns {
		names = append(names, column.Name)
	}

	return names
}

func (r *Schema) GetColumns(table string) ([]driver.Column, error) {
	state := r.state()
	var dbColumns []driver.DBColumn
	sql, err := state.grammar.CompileColumns(state.schema, table)
	if err != nil {
		return nil, err
	}

	if err := state.orm.Query().Raw(sql).Scan(&dbColumns); err != nil {
		return nil, err
	}

	return state.processor.ProcessColumns(dbColumns), nil
}

func (r *Schema) GetConnection() string {
	r.mu.RLock()
	connection := r.connection
	resolverMode := r.resolveOrm != nil
	orm := r.orm
	r.mu.RUnlock()

	if resolverMode {
		return connection
	}

	return orm.Name()
}

func (r *Schema) GetForeignKeys(table string) ([]driver.ForeignKey, error) {
	state := r.state()
	table = state.prefix + table

	var dbForeignKeys []driver.DBForeignKey
	if err := state.orm.Query().Raw(state.grammar.CompileForeignKeys(state.schema, table)).Scan(&dbForeignKeys); err != nil {
		return nil, err
	}

	return state.processor.ProcessForeignKeys(dbForeignKeys), nil
}

func (r *Schema) GetIndexListing(table string) []string {
	indexes, err := r.GetIndexes(table)
	if err != nil {
		r.log.Errorf("failed to get %s indexes: %v", table, err)
		return nil
	}

	var names []string
	for _, index := range indexes {
		names = append(names, index.Name)
	}

	return names
}

func (r *Schema) GetIndexes(table string) ([]driver.Index, error) {
	state := r.state()
	var dbIndexes []driver.DBIndex
	sql, err := state.grammar.CompileIndexes(state.schema, table)
	if err != nil {
		return nil, err
	}

	if err := state.orm.Query().Raw(sql).Scan(&dbIndexes); err != nil {
		return nil, err
	}

	return state.processor.ProcessIndexes(dbIndexes), nil
}

// GetModel retrieves a registered model by name. If the name doesn't contain a package
// (no dot), it automatically prepends "models." to the name. Returns nil if the model
// is not found in the registry.
func (r *Schema) GetModel(name string) any {
	// If no dot, assume "models" package
	if !strings.Contains(name, ".") {
		name = "models." + name
	}

	return r.modelsByFullName[name]
}

func (r *Schema) GetTableListing() []string {
	tables, err := r.GetTables()
	if err != nil {
		r.log.Errorf("failed to get tables: %v", err)
		return nil
	}

	var names []string
	for _, table := range tables {
		names = append(names, table.Name)
	}

	return names
}

func (r *Schema) GetTables() ([]driver.Table, error) {
	return r.tables(r.state())
}

func (r *Schema) tables(state connectionState) ([]driver.Table, error) {
	var tables []driver.Table
	if err := state.orm.Query().Raw(state.grammar.CompileTables(state.orm.DatabaseName())).Scan(&tables); err != nil {
		return nil, err
	}

	return tables, nil
}

func (r *Schema) GetTypes() ([]driver.Type, error) {
	return r.types(r.state())
}

func (r *Schema) types(state connectionState) ([]driver.Type, error) {
	var types []driver.Type
	if err := state.orm.Query().Raw(state.grammar.CompileTypes()).Scan(&types); err != nil {
		return nil, err
	}

	return state.processor.ProcessTypes(types), nil
}

func (r *Schema) GetViews() ([]driver.View, error) {
	return r.views(r.state())
}

func (r *Schema) views(state connectionState) ([]driver.View, error) {
	var views []driver.View
	if err := state.orm.Query().Raw(state.grammar.CompileViews(state.orm.DatabaseName())).Scan(&views); err != nil {
		return nil, err
	}

	return views, nil
}

func (r *Schema) GoTypes() []contractsschema.GoType {
	return r.goTypes
}

func (r *Schema) HasColumn(table, column string) bool {
	return slices.Contains(r.GetColumnListing(table), column)
}

func (r *Schema) HasColumns(table string, columns []string) bool {
	columnListing := r.GetColumnListing(table)
	for _, column := range columns {
		if !slices.Contains(columnListing, column) {
			return false
		}
	}

	return true
}

func (r *Schema) HasIndex(table, index string) bool {
	indexListing := r.GetIndexListing(table)

	return slices.Contains(indexListing, index)
}

func (r *Schema) HasTable(name string) bool {
	state := r.state()
	var schema string
	if strings.Contains(name, ".") {
		lastDotIndex := strings.LastIndex(name, ".")
		schema = name[:lastDotIndex]
		name = name[lastDotIndex+1:]
	}

	tableName := state.prefix + name

	tables, err := r.tables(state)
	if err != nil {
		r.log.Errorf(errors.SchemaFailedToGetTables.Args(state.orm.Name(), err).Error())
		return false
	}

	for _, table := range tables {
		if table.Name == tableName {
			if schema == "" || schema == table.Schema {
				return true
			}
		}
	}

	return false
}

func (r *Schema) HasType(name string) bool {
	state := r.state()
	types, err := r.types(state)
	if err != nil {
		r.log.Errorf(errors.SchemaFailedToGetTables.Args(state.orm.Name(), err).Error())
		return false
	}

	for _, t := range types {
		if t.Name == name {
			return true
		}
	}

	return false
}

func (r *Schema) HasView(name string) bool {
	state := r.state()
	views, err := r.views(state)
	if err != nil {
		r.log.Errorf(errors.SchemaFailedToGetTables.Args(state.orm.Name(), err).Error())
		return false
	}

	for _, view := range views {
		if view.Name == name {
			return true
		}
	}

	return false
}

func (r *Schema) Migrations() []contractsschema.Migration {
	return r.migrations
}

func (r *Schema) Orm() contractsorm.Orm {
	return r.state().orm
}

func (r *Schema) Prune() error {
	state := r.state()
	if sql := state.grammar.CompilePrune(state.orm.DatabaseName()); len(sql) > 0 {
		_, err := state.orm.Query().Exec(sql)

		return err
	}

	return nil
}

func (r *Schema) Register(migrations []contractsschema.Migration) {
	existingSignatures := make(map[string]bool)

	for _, migration := range migrations {
		signature := migration.Signature()

		if existingSignatures[signature] {
			color.Errorf("Duplicate migration signature: %s in %T\n", signature, migration)
		} else {
			existingSignatures[signature] = true
			r.migrations = append(r.migrations, migration)
		}
	}
}

func (r *Schema) Rename(from, to string) error {
	blueprint := r.createBlueprint(from)
	blueprint.Rename(to)

	if err := r.build(blueprint); err != nil {
		return errors.SchemaFailedToRenameTable.Args(from, err)
	}

	return nil
}

// SetConnection switches the schema to a connection. In resolver mode, a switch to the build
// connection restores the values kept at build time and the resolver mode. A switch to any other
// connection derives a new Orm from the Orm resolved at that moment and keeps it until the next
// switch. If the schema stays switched when Orm.Fresh closes the pool of its derived Orm, the
// caller must switch the schema again before using it.
func (r *Schema) SetConnection(name string) {
	connection := r.connectionName(name)
	if r.resolveOrm != nil && connection == r.buildConnection {
		r.mu.Lock()
		defer r.mu.Unlock()

		r.connection = r.buildConnection
		r.driver = r.buildState.driver
		r.grammar = r.buildState.grammar
		r.orm = nil
		r.prefix = r.buildState.prefix
		r.processor = r.buildState.processor
		r.schema = r.buildState.schema

		return
	}

	schema, err := r.newConnection(connection)
	if err != nil {
		r.log.Panic(errors.SchemaConnectionNotFound.Args(name).SetModule(errors.ModuleSchedule).Error())
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.connection = connection
	r.driver = schema.driver
	r.grammar = schema.grammar
	r.orm = schema.orm
	r.prefix = schema.prefix
	r.processor = schema.processor
	r.schema = schema.schema
}

func (r *Schema) newConnection(name string) (*Schema, error) {
	connection := r.connectionName(name)
	connectionDriver, err := r.driverForConnection(connection)
	if err != nil {
		return nil, err
	}

	schema, err := NewSchema(r.config, r.log, r.currentOrm().Connection(connection), connectionDriver, r.migrations)
	if err != nil {
		return nil, err
	}

	schema.goTypes = slices.Clone(r.goTypes)
	schema.models = slices.Clone(r.models)
	schema.modelsByFullName = make(map[string]any, len(r.modelsByFullName))
	maps.Copy(schema.modelsByFullName, r.modelsByFullName)

	return schema, nil
}

// currentOrm returns the Orm that a new connection derives from. With a resolver it is the Orm
// resolved now, also when the schema is switched, so a switch after Orm.Fresh does not derive
// from a closed pool. Without a resolver it is the Orm of the schema.
func (r *Schema) currentOrm() contractsorm.Orm {
	if r.resolveOrm != nil {
		return r.resolveOrm()
	}

	return r.state().orm
}

func (r *Schema) connectionName(name string) string {
	if name != "" {
		return name
	}

	return r.config.GetString("database.default")
}

func (r *Schema) driverForConnection(name string) (driver.Driver, error) {
	driverCallback, exist := r.config.Get(fmt.Sprintf("database.connections.%s.via", name)).(func() (driver.Driver, error))
	if !exist {
		return nil, errors.DatabaseConfigNotFound
	}

	return driverCallback()
}

func (r *Schema) Sql(sql string) error {
	state := r.state()
	_, err := state.orm.Query().Exec(sql)

	return err
}

func (r *Schema) Table(table string, callback func(table contractsschema.Blueprint)) error {
	blueprint := r.createBlueprint(table)
	callback(blueprint)

	if err := r.build(blueprint); err != nil {
		return errors.SchemaFailedToChangeTable.Args(table, err)
	}

	return nil
}

func (r *Schema) build(blueprint contractsschema.Blueprint) error {
	state := r.state()
	if state.orm.Query().InTransaction() {
		return blueprint.Build(state.orm.Query(), state.grammar)
	}

	return state.orm.Transaction(func(tx contractsorm.Query) error {
		return blueprint.Build(tx, state.grammar)
	})
}

func (r *Schema) createBlueprint(table string) contractsschema.Blueprint {
	state := r.state()
	return NewBlueprint(r, state.prefix, table)
}

// extendGoTypes merges user-provided GoType overrides and additions into the schema's default mappings.
// New patterns (not present in defaults) are prepended for highest priority. Existing patterns are updated with non-zero override fields.
func (r *Schema) extendGoTypes(overrides []contractsschema.GoType) {
	if len(overrides) == 0 {
		return
	}

	defaults := r.goTypes
	defaultPatterns := make(map[string]bool, len(defaults))
	for _, d := range defaults {
		defaultPatterns[d.Pattern] = true
	}

	overrideMap := make(map[string]contractsschema.GoType, len(overrides))
	var newPatterns []contractsschema.GoType
	for _, o := range overrides {
		overrideMap[o.Pattern] = o
		if !defaultPatterns[o.Pattern] {
			newPatterns = append(newPatterns, o)
		}
	}

	result := make([]contractsschema.GoType, 0, len(defaults)+len(newPatterns))
	result = append(result, newPatterns...)

	for _, d := range defaults {
		if o, exists := overrideMap[d.Pattern]; exists {
			if o.Type != "" {
				d.Type = o.Type
			}
			if o.NullType != "" {
				d.NullType = o.NullType
			}
			if o.Import != "" {
				d.Import = o.Import
			}
			if o.NullImport != "" {
				d.NullImport = o.NullImport
			}
		}
		result = append(result, d)
	}

	r.goTypes = result
}

func (r *Schema) extendModels(models []any) {
	for _, m := range models {
		fullName := getModelName(m)
		if fullName == "" {
			continue
		}

		// Use full name for duplicate detection
		if _, exists := r.modelsByFullName[fullName]; exists {
			continue
		}

		r.models = append(r.models, m)
		r.modelsByFullName[fullName] = m
	}
}

// modelType returns the reflect.Type for a model, dereferencing pointers.
func modelType(m any) reflect.Type {
	if m == nil {
		return nil
	}
	t := reflect.TypeOf(m)
	if t.Kind() == reflect.Pointer {
		return t.Elem()
	}
	return t
}

// getModelName returns package.TypeName (e.g., "models.User").
func getModelName(m any) string {
	t := modelType(m)
	if t == nil {
		return ""
	}
	if pkg := t.PkgPath(); pkg != "" {
		if i := strings.LastIndexByte(pkg, '/'); i >= 0 {
			pkg = pkg[i+1:]
		}
		return pkg + "." + t.Name()
	}
	return t.Name()
}

func defaultGoTypes() []contractsschema.GoType {
	return []contractsschema.GoType{
		// Special cases first - these need to be matched before general patterns
		{Pattern: "(?i)^tinyint\\(1\\)$", Type: "bool", NullType: "*bool"}, // MySQL boolean representation

		// Boolean types
		{Pattern: "(?i)^bool(ean)?$", Type: "bool", NullType: "*bool"},
		{Pattern: "(?i)^bit(\\(1\\))?$", Type: "bool", NullType: "*bool"}, // Single bit as boolean

		// Integer types - ordered from most specific to general
		// Unsigned variants (MySQL)
		{Pattern: "(?i)^bigint\\s+unsigned(\\(\\d+\\))?$", Type: "uint64", NullType: "*uint64"},
		{Pattern: "(?i)^int(eger)?\\s+unsigned(\\(\\d+\\))?$", Type: "uint32", NullType: "*uint32"},
		{Pattern: "(?i)^mediumint\\s+unsigned(\\(\\d+\\))?$", Type: "uint32", NullType: "*uint32"},
		{Pattern: "(?i)^smallint\\s+unsigned(\\(\\d+\\))?$", Type: "uint16", NullType: "*uint16"},
		{Pattern: "(?i)^tinyint\\s+unsigned(\\(\\d+\\))?$", Type: "uint8", NullType: "*uint8"},

		// PostgreSQL serials
		{Pattern: "(?i)^bigserial(\\(\\d+\\))?$", Type: "int64", NullType: "*int64"},
		{Pattern: "(?i)^serial(\\(\\d+\\))?$", Type: "int", NullType: "*int"},
		{Pattern: "(?i)^smallserial(\\(\\d+\\))?$", Type: "int16", NullType: "*int16"},

		// Standard integer types
		{Pattern: "(?i)^bigint(\\(\\d+\\))?$", Type: "int64", NullType: "*int64"},
		{Pattern: "(?i)^int8(\\(\\d+\\))?$", Type: "int64", NullType: "*int64"},      // PostgreSQL
		{Pattern: "(?i)^mediumint(\\(\\d+\\))?$", Type: "int32", NullType: "*int32"}, // MySQL
		{Pattern: "(?i)^int(eger)?(\\(\\d+\\))?$", Type: "int", NullType: "*int"},
		{Pattern: "(?i)^int4(\\(\\d+\\))?$", Type: "int32", NullType: "*int32"}, // PostgreSQL
		{Pattern: "(?i)^smallint(\\(\\d+\\))?$", Type: "int16", NullType: "*int16"},
		{Pattern: "(?i)^int2(\\(\\d+\\))?$", Type: "int16", NullType: "*int16"},  // PostgreSQL
		{Pattern: "(?i)^tinyint(\\(\\d+\\))?$", Type: "int8", NullType: "*int8"}, // MySQL (when not tinyint(1))
		{Pattern: "(?i)^year(\\(\\d+\\))?$", Type: "int16", NullType: "*int16"},  // MySQL YEAR type

		// Bit/binary integer types
		{Pattern: "(?i)^bit(\\(\\d+\\))?$", Type: "[]byte", NullType: "[]byte"}, // Multi-bit

		// Fixed-precision types
		{Pattern: "(?i)^money(\\(\\d+,?\\d*\\))?$", Type: "float64", NullType: "*float64"}, // PostgreSQL, SQL Server
		{Pattern: "(?i)^decimal(\\(\\d+,?\\d*\\))?$", Type: "float64", NullType: "*float64"},
		{Pattern: "(?i)^dec(\\(\\d+,?\\d*\\))?$", Type: "float64", NullType: "*float64"},
		{Pattern: "(?i)^numeric(\\(\\d+,?\\d*\\))?$", Type: "float64", NullType: "*float64"},
		{Pattern: "(?i)^fixed(\\(\\d+,?\\d*\\))?$", Type: "float64", NullType: "*float64"}, // MySQL

		// Floating point types
		{Pattern: "(?i)^double precision(\\(\\d+,?\\d*\\))?$", Type: "float64", NullType: "*float64"},
		{Pattern: "(?i)^double(\\(\\d+,?\\d*\\))?$", Type: "float64", NullType: "*float64"},
		{Pattern: "(?i)^float8(\\(\\d+,?\\d*\\))?$", Type: "float64", NullType: "*float64"}, // PostgreSQL
		{Pattern: "(?i)^float4(\\(\\d+,?\\d*\\))?$", Type: "float32", NullType: "*float32"}, // PostgreSQL
		{Pattern: "(?i)^float(\\(\\d+,?\\d*\\))?$", Type: "float32", NullType: "*float32"},  // MySQL
		{Pattern: "(?i)^real(\\(\\d+,?\\d*\\))?$", Type: "float32", NullType: "*float32"},

		// String types - longer/specific types first
		{Pattern: "(?i)^character\\s+varying(\\(\\d+\\))?$", Type: "string", NullType: "*string"},
		{Pattern: "(?i)^varchar(\\(\\d+\\))?$", Type: "string", NullType: "*string"},
		{Pattern: "(?i)^character(\\(\\d+\\))?$", Type: "string", NullType: "*string"},
		{Pattern: "(?i)^nvarchar(\\(\\d+\\))?$", Type: "string", NullType: "*string"}, // SQL Server
		{Pattern: "(?i)^nchar(\\(\\d+\\))?$", Type: "string", NullType: "*string"},    // SQL Server
		{Pattern: "(?i)^national\\s+char(acter)?(\\(\\d+\\))?$", Type: "string", NullType: "*string"},
		{Pattern: "(?i)^national\\s+varchar(\\(\\d+\\))?$", Type: "string", NullType: "*string"},
		{Pattern: "(?i)^longtext$", Type: "string", NullType: "*string"},   // MySQL
		{Pattern: "(?i)^mediumtext$", Type: "string", NullType: "*string"}, // MySQL
		{Pattern: "(?i)^tinytext$", Type: "string", NullType: "*string"},   // MySQL
		{Pattern: "(?i)^ntext$", Type: "string", NullType: "*string"},      // SQL Server
		{Pattern: "(?i)^text(\\(\\d+\\))?$", Type: "string", NullType: "*string"},
		{Pattern: "(?i)^char(\\(\\d+\\))?$", Type: "string", NullType: "*string"},
		{Pattern: "(?i)^varchar2(\\(\\d+\\))?$", Type: "string", NullType: "*string"},  // Oracle
		{Pattern: "(?i)^nvarchar2(\\(\\d+\\))?$", Type: "string", NullType: "*string"}, // Oracle
		{Pattern: "(?i)^citext$", Type: "string", NullType: "*string"},                 // PostgreSQL

		// JSON types
		{Pattern: "(?i)^jsonb$", Type: "string", NullType: "*string"}, // PostgreSQL
		{Pattern: "(?i)^json$", Type: "string", NullType: "*string"},

		// Date and Time types
		{Pattern: "(?i)^timestamptz(\\(\\d+\\))?$", Type: "carbon.DateTime", NullType: "*carbon.DateTime", Import: "github.com/goravel/framework/support/carbon"},                             // PostgreSQL
		{Pattern: "(?i)^timestamp(\\(\\d+\\))?\\s+with(out)?\\s+time\\s+zone$", Type: "carbon.DateTime", NullType: "*carbon.DateTime", Import: "github.com/goravel/framework/support/carbon"}, // PostgreSQL
		{Pattern: "(?i)^timestamp(\\(\\d+\\))?$", Type: "carbon.DateTime", NullType: "*carbon.DateTime", Import: "github.com/goravel/framework/support/carbon"},
		{Pattern: "(?i)^datetime(\\(\\d+\\))?$", Type: "carbon.DateTime", NullType: "*carbon.DateTime", Import: "github.com/goravel/framework/support/carbon"},                           // MySQL
		{Pattern: "(?i)^datetime2(\\(\\d+\\))?$", Type: "carbon.DateTime", NullType: "*carbon.DateTime", Import: "github.com/goravel/framework/support/carbon"},                          // SQL Server
		{Pattern: "(?i)^datetimeoffset(\\(\\d+\\))?$", Type: "carbon.DateTime", NullType: "*carbon.DateTime", Import: "github.com/goravel/framework/support/carbon"},                     // SQL Server
		{Pattern: "(?i)^smalldatetime$", Type: "carbon.DateTime", NullType: "*carbon.DateTime", Import: "github.com/goravel/framework/support/carbon"},                                   // SQL Server
		{Pattern: "(?i)^timetz(\\(\\d+\\))?$", Type: "carbon.DateTime", NullType: "*carbon.DateTime", Import: "github.com/goravel/framework/support/carbon"},                             // PostgreSQL
		{Pattern: "(?i)^time(\\(\\d+\\))?\\s+with(out)?\\s+time\\s+zone$", Type: "carbon.DateTime", NullType: "*carbon.DateTime", Import: "github.com/goravel/framework/support/carbon"}, // PostgreSQL
		{Pattern: "(?i)^time(\\(\\d+\\))?$", Type: "carbon.DateTime", NullType: "*carbon.DateTime", Import: "github.com/goravel/framework/support/carbon"},
		{Pattern: "(?i)^date$", Type: "carbon.DateTime", NullType: "*carbon.DateTime", Import: "github.com/goravel/framework/support/carbon"},
		{Pattern: "(?i)^interval$", Type: "string", NullType: "*string"}, // PostgreSQL

		// Range types (PostgreSQL)
		{Pattern: "(?i)^int4range$", Type: "string", NullType: "*string"},
		{Pattern: "(?i)^int8range$", Type: "string", NullType: "*string"},
		{Pattern: "(?i)^numrange$", Type: "string", NullType: "*string"},
		{Pattern: "(?i)^tsrange$", Type: "string", NullType: "*string"},
		{Pattern: "(?i)^tstzrange$", Type: "string", NullType: "*string"},
		{Pattern: "(?i)^daterange$", Type: "string", NullType: "*string"},

		// Enum types
		{Pattern: "(?i)^enum\\([^)]*\\)$", Type: "string", NullType: "*string"}, // MySQL
		{Pattern: "(?i)^set\\([^)]*\\)$", Type: "string", NullType: "*string"},  // MySQL

		// Binary types - larger types first
		{Pattern: "(?i)^longblob$", Type: "[]byte", NullType: "[]byte"},               // MySQL
		{Pattern: "(?i)^mediumblob$", Type: "[]byte", NullType: "[]byte"},             // MySQL
		{Pattern: "(?i)^tinyblob$", Type: "[]byte", NullType: "[]byte"},               // MySQL
		{Pattern: "(?i)^blob(\\(\\d+\\))?$", Type: "[]byte", NullType: "[]byte"},      // MySQL
		{Pattern: "(?i)^image$", Type: "[]byte", NullType: "[]byte"},                  // SQL Server
		{Pattern: "(?i)^varbinary(\\(\\d+\\))?$", Type: "[]byte", NullType: "[]byte"}, // MySQL/SQL Server
		{Pattern: "(?i)^binary(\\(\\d+\\))?$", Type: "[]byte", NullType: "[]byte"},    // MySQL/SQL Server
		{Pattern: "(?i)^bytea$", Type: "[]byte", NullType: "[]byte"},                  // PostgreSQL

		// Network types (PostgreSQL)
		{Pattern: "(?i)^macaddr8$", Type: "string", NullType: "*string"},
		{Pattern: "(?i)^macaddr$", Type: "string", NullType: "*string"},
		{Pattern: "(?i)^cidr$", Type: "string", NullType: "*string"},
		{Pattern: "(?i)^inet$", Type: "string", NullType: "*string"},

		// Geometric types (PostgreSQL)
		{Pattern: "(?i)^circle$", Type: "string", NullType: "*string"},
		{Pattern: "(?i)^polygon$", Type: "string", NullType: "*string"},
		{Pattern: "(?i)^path$", Type: "string", NullType: "*string"},
		{Pattern: "(?i)^box$", Type: "string", NullType: "*string"},
		{Pattern: "(?i)^lseg$", Type: "string", NullType: "*string"},
		{Pattern: "(?i)^line$", Type: "string", NullType: "*string"},
		{Pattern: "(?i)^point$", Type: "string", NullType: "*string"},

		// UUID/GUID types
		{Pattern: "(?i)^uuid$", Type: "string", NullType: "*string"},             // PostgreSQL
		{Pattern: "(?i)^uniqueidentifier$", Type: "string", NullType: "*string"}, // SQL Server

		// XML and other text types
		{Pattern: "(?i)^xml$", Type: "string", NullType: "*string"},
		{Pattern: "(?i)^rowversion$", Type: "[]byte", NullType: "[]byte"}, // SQL Server

		// Spatial/Geometry types
		{Pattern: "(?i)^geometry$", Type: "string", NullType: "*string"},    // MySQL/PostgreSQL/SQL Server
		{Pattern: "(?i)^geography$", Type: "string", NullType: "*string"},   // SQL Server
		{Pattern: "(?i)^st_geometry$", Type: "string", NullType: "*string"}, // PostgreSQL PostGIS

		// Miscellaneous specialized types
		{Pattern: "(?i)^hstore$", Type: "map[string]string", NullType: "*map[string]string"}, // PostgreSQL
		{Pattern: "(?i)^hierarchyid$", Type: "string", NullType: "*string"},                  // SQL Server

		// SQLite specific
		{Pattern: "(?i)^rowid$", Type: "int64", NullType: "*int64"}, // SQLite

		// Array types (PostgreSQL)
		{Pattern: "(?i)^(.+)\\[\\]$", Type: "string", NullType: "*string"}, // Match any array type like "text[]"

		// Fallback for unknown types
		{Pattern: ".*", Type: "any", NullType: "any"},
	}
}
