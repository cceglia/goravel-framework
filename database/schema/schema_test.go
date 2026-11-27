package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	contractsdatabase "github.com/goravel/framework/contracts/database"
	contractsdb "github.com/goravel/framework/contracts/database/db"
	contractsdriver "github.com/goravel/framework/contracts/database/driver"
	contractsorm "github.com/goravel/framework/contracts/database/orm"
	contractsschema "github.com/goravel/framework/contracts/database/schema"
	"github.com/goravel/framework/errors"
	mocksconfig "github.com/goravel/framework/mocks/config"
	mocksdriver "github.com/goravel/framework/mocks/database/driver"
	mocksorm "github.com/goravel/framework/mocks/database/orm"
	mockslog "github.com/goravel/framework/mocks/log"
)

type User struct {
	ID int64 `gorm:"primaryKey"`
}

type Address struct {
	ID int64 `gorm:"primaryKey"`
}

type Relation struct {
	UserID    int64 `gorm:"primaryKey"`
	AddressID int64 `gorm:"primaryKey"`
}

func defaultModels() []any {
	return []any{&User{}}
}

type SchemaTestSuite struct {
	suite.Suite
}

func TestSchemaTestSuite(t *testing.T) {
	suite.Run(t, new(SchemaTestSuite))
}

func (r *SchemaTestSuite) TestExtendGoTypes() {
	defaultLen := len(defaultGoTypes())
	tests := []struct {
		name      string
		overrides []contractsschema.GoType
		assert    func(schema *Schema)
	}{
		{
			name:      "empty_overrides",
			overrides: nil,
			assert: func(schema *Schema) {
				r.Equal(defaultLen, len(schema.goTypes), "goTypes length should remain unchanged with nil overrides")
			},
		},
		{
			name: "add_single_new_type",
			overrides: []contractsschema.GoType{
				{Pattern: "custom_type", Type: "CustomType", NullType: "*CustomType", Import: "github.com/custom/pkg"},
			},
			assert: func(schema *Schema) {
				r.Equal(defaultLen+1, len(schema.goTypes), "goTypes length should increase by 1")

				customType, found := findGoTypeByPattern("custom_type", schema.goTypes)
				r.True(found, "custom_type should be found in goTypes")
				r.Equal("CustomType", customType.Type, "Type should match")
				r.Equal("*CustomType", customType.NullType, "NullType should match")
				r.Equal("github.com/custom/pkg", customType.Import, "Import should match")
			},
		},
		{
			name: "add_multiple_new_types",
			overrides: []contractsschema.GoType{
				{Pattern: "type1", Type: "Type1", NullType: "*Type1", Import: "pkg1"},
				{Pattern: "type2", Type: "Type2", NullType: "*Type2", Import: "pkg2"},
			},
			assert: func(schema *Schema) {
				r.Equal(defaultLen+2, len(schema.goTypes), "goTypes length should increase by 2")

				type1, found := findGoTypeByPattern("type1", schema.goTypes)
				r.True(found, "type1 should be found in goTypes")
				r.Equal("Type1", type1.Type, "Type should match for type1")

				type2, found := findGoTypeByPattern("type2", schema.goTypes)
				r.True(found, "type2 should be found in goTypes")
				r.Equal("Type2", type2.Type, "Type should match for type2")
			},
		},
		{
			name: "override_existing_type_fully",
			overrides: []contractsschema.GoType{
				{Pattern: "(?i)^uuid$", Type: "custom.UUID", NullType: "*custom.UUID", Import: "custom/uuid", NullImport: "custom/uuid/null"},
			},
			assert: func(schema *Schema) {
				r.Equal(defaultLen, len(schema.goTypes), "goTypes length should remain unchanged when overriding")

				// Find the overridden type
				uuidType, found := findGoTypeByPattern("(?i)^uuid$", schema.goTypes)
				r.True(found, "uuid type should be found in goTypes")
				r.Equal("custom.UUID", uuidType.Type, "Type should be overridden")
				r.Equal("*custom.UUID", uuidType.NullType, "NullType should be overridden")
				r.Equal("custom/uuid", uuidType.Import, "Import should be overridden")
				r.Equal("custom/uuid/null", uuidType.NullImport, "NullImport should be overridden")
			},
		},
		{
			name: "override_existing_type_partially",
			overrides: []contractsschema.GoType{
				{Pattern: "(?i)^timestamp$", Type: "time.Time", NullType: "", Import: "time"},
			},
			assert: func(schema *Schema) {
				timestampType, found := findGoTypeByPattern("(?i)^timestamp$", schema.goTypes)
				r.True(found, "timestamp type should be found in goTypes")
				r.Equal("time.Time", timestampType.Type, "Type should be overridden")
				r.Equal("time", timestampType.Import, "Import should be overridden")

				originalTimestamp, _ := findGoTypeByPattern("(?i)^timestamp$", defaultGoTypes())
				r.Equal(originalTimestamp.NullType, timestampType.NullType, "NullType should remain unchanged")
			},
		},
		{
			name: "mix_new_and_override",
			overrides: []contractsschema.GoType{
				{Pattern: "new_pattern", Type: "NewType", NullType: "*NewType"},
				{Pattern: "(?i)^json$", Type: "CustomJSON", NullType: "*CustomJSON"},
			},
			assert: func(schema *Schema) {
				r.Equal(defaultLen+1, len(schema.goTypes), "goTypes length should increase by 1")

				// Check new type
				newType, found := findGoTypeByPattern("new_pattern", schema.goTypes)
				r.True(found, "new_pattern should be found in goTypes")
				r.Equal("NewType", newType.Type, "Type should match for new pattern")

				// Check overridden type
				jsonType, found := findGoTypeByPattern("(?i)^json$", schema.goTypes)
				r.True(found, "json type should be found in goTypes")
				r.Equal("CustomJSON", jsonType.Type, "Type should be overridden for json")
				r.Equal("*CustomJSON", jsonType.NullType, "NullType should be overridden for json")
			},
		},
		{
			name: "multiple_overrides_for_same_pattern",
			overrides: []contractsschema.GoType{
				{Pattern: "(?i)^uuid$", Type: "FirstUUID", NullType: "*FirstUUID"},
				{Pattern: "(?i)^uuid$", Type: "LastUUID", NullType: "*LastUUID"},
			},
			assert: func(schema *Schema) {
				uuidType, found := findGoTypeByPattern("(?i)^uuid$", schema.goTypes)
				r.True(found, "uuid type should be found in goTypes")
				r.Equal("LastUUID", uuidType.Type, "Type should be set to the last override value")
				r.Equal("*LastUUID", uuidType.NullType, "NullType should be set to the last override value")
			},
		},
	}

	for _, test := range tests {
		r.Run(test.name, func() {
			schema := getSchema()
			schema.extendGoTypes(test.overrides)
			test.assert(schema)
		})
	}
}

func (r *SchemaTestSuite) TestExtendModels() {
	defaultLen := len(defaultModels())
	tests := []struct {
		name      string
		overrides []any
		assert    func(schema *Schema)
	}{
		{
			name:      "empty_overrides",
			overrides: nil,
			assert: func(schema *Schema) {
				r.Equal(defaultLen, len(schema.models), "models length should remain unchanged with nil overrides")
			},
		},
		{
			name:      "add_single_new_model",
			overrides: []any{&Address{}},
			assert: func(schema *Schema) {
				r.Equal(defaultLen+1, len(schema.models), "models length should increase by 1")

				addressModel := schema.GetModel("schema.Address")
				r.NotNil(addressModel, "Address model should not be nil")
			},
		},
		{
			name:      "add_multiple_new_models",
			overrides: []any{&Address{}, &Relation{}},
			assert: func(schema *Schema) {
				r.Equal(defaultLen+2, len(schema.models), "models length should increase by 2")

				addressModel := schema.GetModel("schema.Address")
				r.NotNil(addressModel, "Address model should not be nil")

				relationModel := schema.GetModel("schema.Relation")
				r.NotNil(relationModel, "Relation model should not be nil")
			},
		},
		{
			name:      "duplicate_model_ignored",
			overrides: []any{&User{}},
			assert: func(schema *Schema) {
				r.Equal(defaultLen, len(schema.models), "models length should remain unchanged for duplicates")

				userModel := schema.GetModel("schema.User")
				r.NotNil(userModel, "User model should not be nil")
			},
		},
	}

	for _, test := range tests {
		r.Run(test.name, func() {
			schema := getSchema()
			schema.extendModels(test.overrides)
			test.assert(schema)
		})
	}
}

func (r *SchemaTestSuite) TestConnectionUsesConnectionDriver() {
	mockConfig := mocksconfig.NewConfig(r.T())
	mockOrm := mocksorm.NewOrm(r.T())
	mockConnectionOrm := mocksorm.NewOrm(r.T())
	mockDriver := mocksdriver.NewDriver(r.T())
	mockGrammar := mocksdriver.NewGrammar(r.T())
	mockProcessor := mocksdriver.NewProcessor(r.T())

	mockConfig.EXPECT().Get("database.connections.analytics.via").Return(func() (contractsdriver.Driver, error) {
		return mockDriver, nil
	}).Once()
	mockOrm.EXPECT().Connection("analytics").Return(mockConnectionOrm).Once()
	mockDriver.EXPECT().Pool().Return(contractsdatabase.Pool{Writers: []contractsdatabase.Config{{Prefix: "tenant_", Schema: "reporting"}}}).Once()
	mockDriver.EXPECT().Grammar().Return(mockGrammar).Once()
	mockDriver.EXPECT().Processor().Return(mockProcessor).Once()

	schema := &Schema{
		config: mockConfig,
		orm:    mockOrm,
	}

	got, ok := schema.Connection("analytics").(*Schema)
	r.Require().True(ok)
	r.Same(mockConnectionOrm, got.orm)
	r.Same(mockDriver, got.driver)
	r.Same(mockGrammar, got.grammar)
	r.Same(mockProcessor, got.processor)
	r.Equal("tenant_", got.prefix)
	r.Equal("reporting", got.schema)
}

func (r *SchemaTestSuite) TestSetConnectionUsesConnectionDriver() {
	mockConfig := mocksconfig.NewConfig(r.T())
	mockOrm := mocksorm.NewOrm(r.T())
	mockConnectionOrm := mocksorm.NewOrm(r.T())
	mockDriver := mocksdriver.NewDriver(r.T())
	mockGrammar := mocksdriver.NewGrammar(r.T())
	mockProcessor := mocksdriver.NewProcessor(r.T())

	mockConfig.EXPECT().Get("database.connections.analytics.via").Return(func() (contractsdriver.Driver, error) {
		return mockDriver, nil
	}).Once()
	mockOrm.EXPECT().Connection("analytics").Return(mockConnectionOrm).Once()
	mockDriver.EXPECT().Pool().Return(contractsdatabase.Pool{Writers: []contractsdatabase.Config{{Prefix: "tenant_", Schema: "reporting"}}}).Once()
	mockDriver.EXPECT().Grammar().Return(mockGrammar).Once()
	mockDriver.EXPECT().Processor().Return(mockProcessor).Once()

	schema := &Schema{
		config: mockConfig,
		orm:    mockOrm,
	}

	schema.SetConnection("analytics")
	r.Same(mockConnectionOrm, schema.orm)
	r.Same(mockDriver, schema.driver)
	r.Same(mockGrammar, schema.grammar)
	r.Same(mockProcessor, schema.processor)
	r.Equal("tenant_", schema.prefix)
	r.Equal("reporting", schema.schema)
}

func (r *SchemaTestSuite) TestHasObjectLogsLookupError() {
	tests := []struct {
		name    string
		compile func(grammar *mocksdriver.Grammar) string
		query   func(schema *Schema) bool
	}{
		{
			name: "HasTable",
			compile: func(grammar *mocksdriver.Grammar) string {
				grammar.EXPECT().CompileTables("app").Return("tables sql").Once()
				return "tables sql"
			},
			query: func(schema *Schema) bool { return schema.HasTable("users") },
		},
		{
			name: "HasType",
			compile: func(grammar *mocksdriver.Grammar) string {
				grammar.EXPECT().CompileTypes().Return("types sql").Once()
				return "types sql"
			},
			query: func(schema *Schema) bool { return schema.HasType("status") },
		},
		{
			name: "HasView",
			compile: func(grammar *mocksdriver.Grammar) string {
				grammar.EXPECT().CompileViews("app").Return("views sql").Once()
				return "views sql"
			},
			query: func(schema *Schema) bool { return schema.HasView("report") },
		},
	}

	for _, tt := range tests {
		r.Run(tt.name, func() {
			mockOrm := mocksorm.NewOrm(r.T())
			mockQuery := mocksorm.NewQuery(r.T())
			mockGrammar := mocksdriver.NewGrammar(r.T())
			mockLog := mockslog.NewLog(r.T())

			sql := tt.compile(mockGrammar)
			mockOrm.EXPECT().DatabaseName().Return("app").Maybe()
			mockOrm.EXPECT().Name().Return("pgsql").Once()
			mockOrm.EXPECT().Query().Return(mockQuery).Once()
			mockQuery.EXPECT().Raw(sql).Return(mockQuery).Once()
			mockQuery.EXPECT().Scan(mock.Anything).Return(assert.AnError).Once()
			mockLog.EXPECT().Errorf(errors.SchemaFailedToGetTables.Args("pgsql", assert.AnError).Error()).Once()

			schema := &Schema{grammar: mockGrammar, log: mockLog, orm: mockOrm}

			r.False(tt.query(schema))
		})
	}
}

func (r *SchemaTestSuite) TestPrune() {
	tests := []struct {
		name        string
		sql         string
		execErr     error
		expectError error
	}{
		{name: "runs the compiled sql", sql: "prune sql"},
		{name: "returns the exec error", sql: "prune sql", execErr: assert.AnError, expectError: assert.AnError},
		{name: "skips the query when the grammar returns no sql", sql: ""},
	}

	for _, tt := range tests {
		r.Run(tt.name, func() {
			mockOrm := mocksorm.NewOrm(r.T())
			mockGrammar := mocksdriver.NewGrammar(r.T())

			mockOrm.EXPECT().DatabaseName().Return("app").Once()
			mockGrammar.EXPECT().CompilePrune("app").Return(tt.sql).Once()
			if tt.sql != "" {
				mockQuery := mocksorm.NewQuery(r.T())
				mockOrm.EXPECT().Query().Return(mockQuery).Once()
				mockQuery.EXPECT().Exec(tt.sql).Return(&contractsdb.Result{}, tt.execErr).Once()
			}

			schema := &Schema{grammar: mockGrammar, orm: mockOrm}

			r.Equal(tt.expectError, schema.Prune())
		})
	}
}

func getSchema() *Schema {
	schema := &Schema{
		goTypes:          defaultGoTypes(),
		modelsByFullName: make(map[string]any),
	}
	schema.Extend(contractsschema.Extension{
		Models: defaultModels(),
	})
	return schema
}

func TestGetModelName(t *testing.T) {
	tests := []struct {
		name  string
		model any
		want  string
	}{
		{"Pointer to struct", &User{}, "schema.User"},
		{"Value struct", User{}, "schema.User"},
		{"Nil", nil, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := getModelName(tc.model)
			assert.Equal(t, tc.want, got)
		})
	}
}

func findGoTypeByPattern(pattern string, types []contractsschema.GoType) (contractsschema.GoType, bool) {
	for _, t := range types {
		if t.Pattern == pattern {
			return t, true
		}
	}
	return contractsschema.GoType{}, false
}

func (r *SchemaTestSuite) newResolverSchema(buildOrm contractsorm.Orm, resolve func() contractsorm.Orm) *Schema {
	mockDriver := mocksdriver.NewDriver(r.T())
	mockDriver.EXPECT().Pool().Return(contractsdatabase.Pool{Writers: []contractsdatabase.Config{{Prefix: "p_", Schema: "app"}}}).Once()
	mockDriver.EXPECT().Grammar().Return(mocksdriver.NewGrammar(r.T())).Once()
	mockDriver.EXPECT().Processor().Return(mocksdriver.NewProcessor(r.T())).Once()

	schema, err := NewSchema(mocksconfig.NewConfig(r.T()), nil, buildOrm, mockDriver, nil, WithOrmResolver(resolve))
	r.Require().NoError(err)

	return schema
}

// switchFixture is a resolver schema built for "primary" plus a config that serves the
// "analytics" and "reports" connections. The config fails on any other lookup.
type switchFixture struct {
	schema       *Schema
	config       *mocksconfig.Config
	buildDriver  *mocksdriver.Driver
	buildGrammar *mocksdriver.Grammar
	buildOrm     *mocksorm.Orm
	current      contractsorm.Orm
}

func (r *SchemaTestSuite) newSwitchFixture() *switchFixture {
	fixture := &switchFixture{
		config:       mocksconfig.NewConfig(r.T()),
		buildDriver:  mocksdriver.NewDriver(r.T()),
		buildGrammar: mocksdriver.NewGrammar(r.T()),
		buildOrm:     mocksorm.NewOrm(r.T()),
	}
	fixture.current = fixture.buildOrm

	fixture.buildOrm.EXPECT().Name().Return("primary").Once()
	fixture.buildDriver.EXPECT().Pool().Return(contractsdatabase.Pool{Writers: []contractsdatabase.Config{{Prefix: "p_", Schema: "app"}}}).Once()
	fixture.buildDriver.EXPECT().Grammar().Return(fixture.buildGrammar).Once()
	fixture.buildDriver.EXPECT().Processor().Return(mocksdriver.NewProcessor(r.T())).Once()

	schema, err := NewSchema(fixture.config, nil, fixture.buildOrm, fixture.buildDriver, nil, WithOrmResolver(func() contractsorm.Orm {
		return fixture.current
	}))
	r.Require().NoError(err)
	fixture.schema = schema

	return fixture
}

// expectConnection serves the config lookup of a connection and returns the Orm that the
// given base Orm derives for it. The driver of the connection uses the given prefix and schema.
func (r *SchemaTestSuite) expectConnection(fixture *switchFixture, base *mocksorm.Orm, name, prefix, schemaName string) *mocksorm.Orm {
	connectionDriver := mocksdriver.NewDriver(r.T())
	connectionDriver.EXPECT().Pool().Return(contractsdatabase.Pool{Writers: []contractsdatabase.Config{{Prefix: prefix, Schema: schemaName}}}).Once()
	connectionDriver.EXPECT().Grammar().Return(mocksdriver.NewGrammar(r.T())).Once()
	connectionDriver.EXPECT().Processor().Return(mocksdriver.NewProcessor(r.T())).Once()
	fixture.config.EXPECT().Get("database.connections." + name + ".via").Return(func() (contractsdriver.Driver, error) {
		return connectionDriver, nil
	}).Once()

	derived := mocksorm.NewOrm(r.T())
	base.EXPECT().Connection(name).Return(derived).Once()

	return derived
}

func (r *SchemaTestSuite) TestDrop_InTransaction() {
	query := mocksorm.NewQuery(r.T())
	query.EXPECT().InTransaction().Return(true).Once()
	query.EXPECT().Exec("drop table users").Return(nil, nil).Once()

	buildOrm := mocksorm.NewOrm(r.T())
	buildOrm.EXPECT().Query().Return(query).Twice()

	grammar := mocksdriver.NewGrammar(r.T())
	grammar.EXPECT().GetAttributeCommands().Return(nil).Once()
	grammar.EXPECT().CompileDrop(mock.Anything).Return("drop table users").Once()

	mockDriver := mocksdriver.NewDriver(r.T())
	mockDriver.EXPECT().Pool().Return(contractsdatabase.Pool{Writers: []contractsdatabase.Config{{}}}).Once()
	mockDriver.EXPECT().Grammar().Return(grammar).Once()
	mockDriver.EXPECT().Processor().Return(nil).Once()

	schema, err := NewSchema(nil, nil, buildOrm, mockDriver, nil)
	r.Require().NoError(err)

	r.NoError(schema.Drop("users"))
}

func (r *SchemaTestSuite) TestResolverCallsPerOperation() {
	tests := []struct {
		name string
		call func(schema *Schema)
	}{
		{name: "HasTable", call: func(schema *Schema) { schema.HasTable("users") }},
		{name: "HasType", call: func(schema *Schema) { schema.HasType("status") }},
		{name: "HasView", call: func(schema *Schema) { schema.HasView("active_users") }},
		{name: "DropAllTables", call: func(schema *Schema) { r.NoError(schema.DropAllTables()) }},
		{name: "DropAllTypes", call: func(schema *Schema) { r.NoError(schema.DropAllTypes()) }},
		{name: "DropAllViews", call: func(schema *Schema) { r.NoError(schema.DropAllViews()) }},
	}

	for _, tt := range tests {
		r.Run(tt.name+" resolves the Orm once", func() {
			buildOrm := mocksorm.NewOrm(r.T())
			buildOrm.EXPECT().Name().Return("primary").Maybe()
			buildOrm.EXPECT().DatabaseName().Return("db").Maybe()
			query := mocksorm.NewQuery(r.T())
			buildOrm.EXPECT().Query().Return(query).Maybe()
			query.EXPECT().Raw(mock.Anything).Return(query).Maybe()
			query.EXPECT().Scan(mock.Anything).Return(nil).Maybe()
			buildOrm.EXPECT().Transaction(mock.Anything).Return(nil).Maybe()

			grammar := mocksdriver.NewGrammar(r.T())
			grammar.EXPECT().CompileTables(mock.Anything).Return("tables").Maybe()
			grammar.EXPECT().CompileTypes().Return("types").Maybe()
			grammar.EXPECT().CompileViews(mock.Anything).Return("views").Maybe()
			grammar.EXPECT().CompileDropAllTables(mock.Anything, mock.Anything).Return(nil).Maybe()
			grammar.EXPECT().CompileDropAllTypes(mock.Anything, mock.Anything).Return(nil).Maybe()
			grammar.EXPECT().CompileDropAllViews(mock.Anything, mock.Anything).Return(nil).Maybe()
			processor := mocksdriver.NewProcessor(r.T())
			processor.EXPECT().ProcessTypes(mock.Anything).Return(nil).Maybe()

			mockDriver := mocksdriver.NewDriver(r.T())
			mockDriver.EXPECT().Pool().Return(contractsdatabase.Pool{Writers: []contractsdatabase.Config{{}}}).Once()
			mockDriver.EXPECT().Grammar().Return(grammar).Once()
			mockDriver.EXPECT().Processor().Return(processor).Once()

			calls := 0
			schema, err := NewSchema(mocksconfig.NewConfig(r.T()), nil, buildOrm, mockDriver, nil, WithOrmResolver(func() contractsorm.Orm {
				calls++
				return buildOrm
			}))
			r.Require().NoError(err)

			tt.call(schema)

			r.Equal(1, calls)
		})
	}
}

func (r *SchemaTestSuite) TestSetConnection_Resolver() {
	r.Run("switching to another connection derives from the resolved Orm and keeps it", func() {
		fixture := r.newSwitchFixture()
		first := r.expectConnection(fixture, fixture.buildOrm, "analytics", "a_", "reporting")

		fixture.schema.SetConnection("analytics")
		r.Same(first, fixture.schema.Orm())
		r.Same(first, fixture.schema.Orm(), "the derived Orm is stable until the next switch")
		r.Equal("analytics", fixture.schema.GetConnection())
		r.Equal("a_", fixture.schema.prefix)
		r.Equal("reporting", fixture.schema.schema)

		// A later resolver result does not replace the derived Orm.
		fixture.current = mocksorm.NewOrm(r.T())
		r.Same(first, fixture.schema.Orm())
	})

	r.Run("each switch derives a new Orm from the Orm resolved at that moment", func() {
		fixture := r.newSwitchFixture()
		first := r.expectConnection(fixture, fixture.buildOrm, "analytics", "a_", "reporting")
		fixture.schema.SetConnection("analytics")

		freshOrm := mocksorm.NewOrm(r.T())
		fixture.current = freshOrm
		second := r.expectConnection(fixture, freshOrm, "analytics", "a_", "reporting")
		fixture.schema.SetConnection("analytics")

		r.NotSame(first, fixture.schema.Orm())
		r.Same(second, fixture.schema.Orm())

		third := r.expectConnection(fixture, freshOrm, "reports", "r_", "other")
		fixture.schema.SetConnection("reports")
		r.Same(third, fixture.schema.Orm(), "a switch from another connection derives from the resolved Orm")
	})

	r.Run("switching back restores resolver mode and the build metadata", func() {
		fixture := r.newSwitchFixture()
		r.expectConnection(fixture, fixture.buildOrm, "analytics", "a_", "reporting")
		fixture.schema.SetConnection("analytics")

		// The config is not asked for the build connection: the kept values are used.
		fixture.schema.SetConnection("primary")

		r.Equal("primary", fixture.schema.GetConnection())
		r.Same(fixture.buildDriver, fixture.schema.driver)
		r.Same(fixture.buildGrammar, fixture.schema.grammar)
		r.NotNil(fixture.schema.processor)
		r.Equal("p_", fixture.schema.prefix)
		r.Equal("app", fixture.schema.schema)

		freshOrm := mocksorm.NewOrm(r.T())
		fixture.current = freshOrm
		r.Same(freshOrm, fixture.schema.Orm())
	})

	r.Run("an empty name switches to the configured default", func() {
		fixture := r.newSwitchFixture()
		r.expectConnection(fixture, fixture.buildOrm, "analytics", "a_", "reporting")
		fixture.schema.SetConnection("analytics")

		fixture.config.EXPECT().GetString("database.default").Return("primary").Once()
		fixture.schema.SetConnection("")

		r.Equal("primary", fixture.schema.GetConnection())
		r.Same(fixture.current, fixture.schema.Orm())
	})

	r.Run("a changed default makes an empty name a switch to another connection", func() {
		fixture := r.newSwitchFixture()
		derived := r.expectConnection(fixture, fixture.buildOrm, "analytics", "a_", "reporting")

		fixture.config.EXPECT().GetString("database.default").Return("analytics").Once()
		fixture.schema.SetConnection("")

		r.Equal("analytics", fixture.schema.GetConnection())
		r.Same(derived, fixture.schema.Orm())
	})

	r.Run("a named connection schema keeps a fixed Orm", func() {
		fixture := r.newSwitchFixture()
		derived := r.expectConnection(fixture, fixture.buildOrm, "analytics", "a_", "reporting")
		derived.EXPECT().Name().Return("analytics").Once()

		named, ok := fixture.schema.Connection("analytics").(*Schema)
		r.Require().True(ok)
		r.Nil(named.resolveOrm)

		fixture.current = mocksorm.NewOrm(r.T())
		r.Same(derived, named.Orm())
		r.Equal("analytics", named.GetConnection())
	})

	r.Run("a named connection schema for the build connection is not in resolver mode", func() {
		fixture := r.newSwitchFixture()
		derived := r.expectConnection(fixture, fixture.buildOrm, "primary", "p_", "app")

		named, ok := fixture.schema.Connection("primary").(*Schema)
		r.Require().True(ok)

		fixture.current = mocksorm.NewOrm(r.T())
		r.Same(derived, named.Orm())
	})
}

func (r *SchemaTestSuite) TestNewSchema_OrmResolver() {
	r.Run("resolves the current Orm on each call", func() {
		buildOrm := mocksorm.NewOrm(r.T())
		buildOrm.EXPECT().Name().Return("primary").Once()
		freshOrm := mocksorm.NewOrm(r.T())
		current := contractsorm.Orm(buildOrm)

		schema := r.newResolverSchema(buildOrm, func() contractsorm.Orm { return current })
		r.Same(buildOrm, schema.Orm())

		current = freshOrm
		r.Same(freshOrm, schema.Orm())
	})

	r.Run("reports the build connection without calling the resolver", func() {
		buildOrm := mocksorm.NewOrm(r.T())
		buildOrm.EXPECT().Name().Return("primary").Once()

		calls := 0
		schema := r.newResolverSchema(buildOrm, func() contractsorm.Orm {
			calls++
			return buildOrm
		})

		r.Equal("primary", schema.GetConnection())
		r.Equal(0, calls)
	})

	r.Run("keeps the driver metadata of the build connection", func() {
		buildOrm := mocksorm.NewOrm(r.T())
		buildOrm.EXPECT().Name().Return("primary").Once()

		schema := r.newResolverSchema(buildOrm, func() contractsorm.Orm { return buildOrm })
		r.Equal("p_", schema.prefix)
		r.Equal("app", schema.schema)
		r.NotNil(schema.driver)
		r.NotNil(schema.grammar)
		r.NotNil(schema.processor)
	})

	r.Run("a nil resolved Orm gives a nil Orm", func() {
		buildOrm := mocksorm.NewOrm(r.T())
		buildOrm.EXPECT().Name().Return("primary").Once()

		schema := r.newResolverSchema(buildOrm, func() contractsorm.Orm { return nil })
		r.Nil(schema.Orm())
	})

	r.Run("a nil resolver function means no resolver", func() {
		buildOrm := mocksorm.NewOrm(r.T())
		mockDriver := mocksdriver.NewDriver(r.T())
		mockDriver.EXPECT().Pool().Return(contractsdatabase.Pool{Writers: []contractsdatabase.Config{{}}}).Once()
		mockDriver.EXPECT().Grammar().Return(nil).Once()
		mockDriver.EXPECT().Processor().Return(nil).Once()
		buildOrm.EXPECT().Name().Return("primary").Once()

		schema, err := NewSchema(nil, nil, buildOrm, mockDriver, nil, WithOrmResolver(nil))
		r.Require().NoError(err)
		r.Same(buildOrm, schema.Orm())
		r.Equal("primary", schema.GetConnection())
	})

	r.Run("without a resolver the Orm is the one given at build time", func() {
		buildOrm := mocksorm.NewOrm(r.T())
		mockDriver := mocksdriver.NewDriver(r.T())
		mockDriver.EXPECT().Pool().Return(contractsdatabase.Pool{Writers: []contractsdatabase.Config{{}}}).Once()
		mockDriver.EXPECT().Grammar().Return(nil).Once()
		mockDriver.EXPECT().Processor().Return(nil).Once()

		schema, err := NewSchema(nil, nil, buildOrm, mockDriver, nil)
		r.Require().NoError(err)
		r.Same(buildOrm, schema.Orm())
	})

	r.Run("a nil Orm or nil driver never gets a resolver", func() {
		resolved := mocksorm.NewOrm(r.T())
		resolve := func() contractsorm.Orm { return resolved }

		noOrm, err := NewSchema(nil, nil, nil, nil, nil, WithOrmResolver(resolve))
		r.Require().NoError(err)
		r.Nil(noOrm.Orm())

		buildOrm := mocksorm.NewOrm(r.T())
		noDriver, err := NewSchema(nil, nil, buildOrm, nil, nil, WithOrmResolver(resolve))
		r.Require().NoError(err)
		r.Same(buildOrm, noDriver.Orm())
	})
}

// fillScan returns a Scan callback that stores values in the destination of the query.
func fillScan[T any](values T) func(dest any) {
	return func(dest any) {
		*dest.(*T) = values
	}
}

// expectTransaction makes the Orm run the transaction callback on a query that executes the
// given statements in order and stops at the first one that fails with execErr.
func (r *SchemaTestSuite) expectTransaction(orm *mocksorm.Orm, sqls []string, execErr error) {
	tx := mocksorm.NewQuery(r.T())
	for _, sql := range sqls {
		if execErr != nil {
			tx.EXPECT().Exec(sql).Return(nil, execErr).Once()
			break
		}
		tx.EXPECT().Exec(sql).Return(&contractsdb.Result{}, nil).Once()
	}

	orm.EXPECT().Transaction(mock.Anything).RunAndReturn(func(callback func(contractsorm.Query) error) error {
		return callback(tx)
	}).Once()
}

func (r *SchemaTestSuite) TestDropAll() {
	tables := []contractsdriver.Table{{Name: "users"}}
	types := []contractsdriver.Type{{Name: "status"}}
	views := []contractsdriver.View{{Name: "active_users"}}
	sqls := []string{"drop one", "drop two"}

	tests := []struct {
		name  string
		setup func(orm *mocksorm.Orm, query *mocksorm.Query, grammar *mocksdriver.Grammar, processor *mocksdriver.Processor)
		call  func(schema *Schema) error
	}{
		{
			name: "DropAllTables",
			setup: func(orm *mocksorm.Orm, query *mocksorm.Query, grammar *mocksdriver.Grammar, _ *mocksdriver.Processor) {
				orm.EXPECT().DatabaseName().Return("db").Once()
				grammar.EXPECT().CompileTables("db").Return("tables sql").Once()
				query.EXPECT().Raw("tables sql").Return(query).Once()
				query.EXPECT().Scan(mock.Anything).Run(fillScan(tables)).Return(nil).Once()
				grammar.EXPECT().CompileDropAllTables("app", tables).Return(sqls).Once()
			},
			call: func(schema *Schema) error { return schema.DropAllTables() },
		},
		{
			name: "DropAllTypes",
			setup: func(_ *mocksorm.Orm, query *mocksorm.Query, grammar *mocksdriver.Grammar, processor *mocksdriver.Processor) {
				grammar.EXPECT().CompileTypes().Return("types sql").Once()
				query.EXPECT().Raw("types sql").Return(query).Once()
				query.EXPECT().Scan(mock.Anything).Run(fillScan(types)).Return(nil).Once()
				processor.EXPECT().ProcessTypes(types).Return(types).Once()
				grammar.EXPECT().CompileDropAllTypes("app", types).Return(sqls).Once()
			},
			call: func(schema *Schema) error { return schema.DropAllTypes() },
		},
		{
			name: "DropAllViews",
			setup: func(orm *mocksorm.Orm, query *mocksorm.Query, grammar *mocksdriver.Grammar, _ *mocksdriver.Processor) {
				orm.EXPECT().DatabaseName().Return("db").Once()
				grammar.EXPECT().CompileViews("db").Return("views sql").Once()
				query.EXPECT().Raw("views sql").Return(query).Once()
				query.EXPECT().Scan(mock.Anything).Run(fillScan(views)).Return(nil).Once()
				grammar.EXPECT().CompileDropAllViews("app", views).Return(sqls).Once()
			},
			call: func(schema *Schema) error { return schema.DropAllViews() },
		},
	}

	for _, tt := range tests {
		for _, execErr := range []error{nil, assert.AnError} {
			name := tt.name + " runs every statement in a transaction"
			if execErr != nil {
				name = tt.name + " stops at the first failing statement"
			}

			r.Run(name, func() {
				orm := mocksorm.NewOrm(r.T())
				query := mocksorm.NewQuery(r.T())
				grammar := mocksdriver.NewGrammar(r.T())
				processor := mocksdriver.NewProcessor(r.T())

				orm.EXPECT().Query().Return(query).Once()
				tt.setup(orm, query, grammar, processor)
				r.expectTransaction(orm, sqls, execErr)

				schema := &Schema{grammar: grammar, orm: orm, processor: processor, schema: "app"}

				r.Equal(execErr, tt.call(schema))
			})
		}
	}
}

func (r *SchemaTestSuite) TestGetColumns() {
	dbColumns := []contractsdriver.DBColumn{{Name: "id"}}
	columns := []contractsdriver.Column{{Name: "id"}}

	tests := []struct {
		name          string
		compileErr    error
		scanErr       error
		expectColumns []contractsdriver.Column
		expectError   error
	}{
		{name: "returns the processed columns", expectColumns: columns},
		{name: "returns the compile error", compileErr: assert.AnError, expectError: assert.AnError},
		{name: "returns the scan error", scanErr: assert.AnError, expectError: assert.AnError},
	}

	for _, tt := range tests {
		r.Run(tt.name, func() {
			orm := mocksorm.NewOrm(r.T())
			query := mocksorm.NewQuery(r.T())
			grammar := mocksdriver.NewGrammar(r.T())
			processor := mocksdriver.NewProcessor(r.T())

			grammar.EXPECT().CompileColumns("app", "users").Return("columns sql", tt.compileErr).Once()
			if tt.compileErr == nil {
				orm.EXPECT().Query().Return(query).Once()
				query.EXPECT().Raw("columns sql").Return(query).Once()
				query.EXPECT().Scan(mock.Anything).Run(fillScan(dbColumns)).Return(tt.scanErr).Once()
			}
			if tt.compileErr == nil && tt.scanErr == nil {
				processor.EXPECT().ProcessColumns(dbColumns).Return(columns).Once()
			}

			schema := &Schema{grammar: grammar, orm: orm, processor: processor, schema: "app"}

			got, err := schema.GetColumns("users")

			r.Equal(tt.expectColumns, got)
			r.Equal(tt.expectError, err)
		})
	}
}

func (r *SchemaTestSuite) TestGetForeignKeys() {
	dbForeignKeys := []contractsdriver.DBForeignKey{{Name: "fk_user"}}
	foreignKeys := []contractsdriver.ForeignKey{{Name: "fk_user"}}

	tests := []struct {
		name       string
		scanErr    error
		expectKeys []contractsdriver.ForeignKey
	}{
		{name: "returns the processed foreign keys of the prefixed table", expectKeys: foreignKeys},
		{name: "returns the scan error", scanErr: assert.AnError},
	}

	for _, tt := range tests {
		r.Run(tt.name, func() {
			orm := mocksorm.NewOrm(r.T())
			query := mocksorm.NewQuery(r.T())
			grammar := mocksdriver.NewGrammar(r.T())
			processor := mocksdriver.NewProcessor(r.T())

			grammar.EXPECT().CompileForeignKeys("app", "p_users").Return("fk sql").Once()
			orm.EXPECT().Query().Return(query).Once()
			query.EXPECT().Raw("fk sql").Return(query).Once()
			query.EXPECT().Scan(mock.Anything).Run(fillScan(dbForeignKeys)).Return(tt.scanErr).Once()
			if tt.scanErr == nil {
				processor.EXPECT().ProcessForeignKeys(dbForeignKeys).Return(foreignKeys).Once()
			}

			schema := &Schema{grammar: grammar, orm: orm, prefix: "p_", processor: processor, schema: "app"}

			got, err := schema.GetForeignKeys("users")

			r.Equal(tt.expectKeys, got)
			r.Equal(tt.scanErr, err)
		})
	}
}

func (r *SchemaTestSuite) TestGetIndexes() {
	dbIndexes := []contractsdriver.DBIndex{{Name: "idx_name"}}
	indexes := []contractsdriver.Index{{Name: "idx_name"}}

	tests := []struct {
		name          string
		compileErr    error
		scanErr       error
		expectIndexes []contractsdriver.Index
		expectError   error
	}{
		{name: "returns the processed indexes", expectIndexes: indexes},
		{name: "returns the compile error", compileErr: assert.AnError, expectError: assert.AnError},
		{name: "returns the scan error", scanErr: assert.AnError, expectError: assert.AnError},
	}

	for _, tt := range tests {
		r.Run(tt.name, func() {
			orm := mocksorm.NewOrm(r.T())
			query := mocksorm.NewQuery(r.T())
			grammar := mocksdriver.NewGrammar(r.T())
			processor := mocksdriver.NewProcessor(r.T())

			grammar.EXPECT().CompileIndexes("app", "users").Return("indexes sql", tt.compileErr).Once()
			if tt.compileErr == nil {
				orm.EXPECT().Query().Return(query).Once()
				query.EXPECT().Raw("indexes sql").Return(query).Once()
				query.EXPECT().Scan(mock.Anything).Run(fillScan(dbIndexes)).Return(tt.scanErr).Once()
			}
			if tt.compileErr == nil && tt.scanErr == nil {
				processor.EXPECT().ProcessIndexes(dbIndexes).Return(indexes).Once()
			}

			schema := &Schema{grammar: grammar, orm: orm, processor: processor, schema: "app"}

			got, err := schema.GetIndexes("users")

			r.Equal(tt.expectIndexes, got)
			r.Equal(tt.expectError, err)
		})
	}
}

func (r *SchemaTestSuite) TestGetTablesTypesAndViews() {
	tables := []contractsdriver.Table{{Name: "users"}}
	types := []contractsdriver.Type{{Name: "status"}}
	views := []contractsdriver.View{{Name: "active_users"}}

	newFixture := func(sql string) (*Schema, *mocksorm.Orm, *mocksorm.Query, *mocksdriver.Grammar, *mocksdriver.Processor) {
		orm := mocksorm.NewOrm(r.T())
		query := mocksorm.NewQuery(r.T())
		grammar := mocksdriver.NewGrammar(r.T())
		processor := mocksdriver.NewProcessor(r.T())

		orm.EXPECT().Query().Return(query).Once()
		query.EXPECT().Raw(sql).Return(query).Once()

		return &Schema{grammar: grammar, orm: orm, processor: processor}, orm, query, grammar, processor
	}

	r.Run("GetTables returns the tables of the database", func() {
		schema, orm, query, grammar, _ := newFixture("tables sql")
		orm.EXPECT().DatabaseName().Return("db").Once()
		grammar.EXPECT().CompileTables("db").Return("tables sql").Once()
		query.EXPECT().Scan(mock.Anything).Run(fillScan(tables)).Return(nil).Once()

		got, err := schema.GetTables()

		r.NoError(err)
		r.Equal(tables, got)
	})

	r.Run("GetTypes returns the processed types", func() {
		schema, _, query, grammar, processor := newFixture("types sql")
		grammar.EXPECT().CompileTypes().Return("types sql").Once()
		query.EXPECT().Scan(mock.Anything).Run(fillScan(types)).Return(nil).Once()
		processor.EXPECT().ProcessTypes(types).Return([]contractsdriver.Type{{Name: "processed"}}).Once()

		got, err := schema.GetTypes()

		r.NoError(err)
		r.Equal([]contractsdriver.Type{{Name: "processed"}}, got)
	})

	r.Run("GetViews returns the views of the database", func() {
		schema, orm, query, grammar, _ := newFixture("views sql")
		orm.EXPECT().DatabaseName().Return("db").Once()
		grammar.EXPECT().CompileViews("db").Return("views sql").Once()
		query.EXPECT().Scan(mock.Anything).Run(fillScan(views)).Return(nil).Once()

		got, err := schema.GetViews()

		r.NoError(err)
		r.Equal(views, got)
	})
}

func (r *SchemaTestSuite) TestSql() {
	tests := []struct {
		name        string
		execErr     error
		expectError error
	}{
		{name: "runs the statement"},
		{name: "returns the exec error", execErr: assert.AnError, expectError: assert.AnError},
	}

	for _, tt := range tests {
		r.Run(tt.name, func() {
			orm := mocksorm.NewOrm(r.T())
			query := mocksorm.NewQuery(r.T())

			orm.EXPECT().Query().Return(query).Once()
			query.EXPECT().Exec("select 1").Return(&contractsdb.Result{}, tt.execErr).Once()

			schema := &Schema{orm: orm}

			r.Equal(tt.expectError, schema.Sql("select 1"))
		})
	}
}

func (r *SchemaTestSuite) TestDrop_OutsideTransaction() {
	tests := []struct {
		name        string
		execErr     error
		expectError error
	}{
		{name: "builds the blueprint on the transaction query"},
		{name: "returns the build error", execErr: assert.AnError, expectError: errors.SchemaFailedToDropTable.Args("users", assert.AnError)},
	}

	for _, tt := range tests {
		r.Run(tt.name, func() {
			orm := mocksorm.NewOrm(r.T())
			query := mocksorm.NewQuery(r.T())
			grammar := mocksdriver.NewGrammar(r.T())

			orm.EXPECT().Query().Return(query).Once()
			query.EXPECT().InTransaction().Return(false).Once()
			grammar.EXPECT().GetAttributeCommands().Return(nil).Once()
			grammar.EXPECT().CompileDrop(mock.Anything).Return("drop table users").Once()
			r.expectTransaction(orm, []string{"drop table users"}, tt.execErr)

			schema := &Schema{grammar: grammar, orm: orm}

			r.Equal(tt.expectError, schema.Drop("users"))
		})
	}
}
