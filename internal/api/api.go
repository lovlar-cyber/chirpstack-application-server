package api

import (
	"github.com/pkg/errors"

	"github.com/lovlar-cyber/chirpstack-application-server/internal/api/as"
	"github.com/lovlar-cyber/chirpstack-application-server/internal/api/external"
	"github.com/lovlar-cyber/chirpstack-application-server/internal/api/js"
	"github.com/lovlar-cyber/chirpstack-application-server/internal/config"
)

// Setup configures the API endpoints.
func Setup(conf config.Config) error {
	if err := as.Setup(conf); err != nil {
		return errors.Wrap(err, "setup application-server api error")
	}

	if err := external.Setup(conf); err != nil {
		return errors.Wrap(err, "setup external api error")
	}

	if err := js.Setup(conf); err != nil {
		return errors.Wrap(err, "setup join-server api error")
	}

	return nil
}
