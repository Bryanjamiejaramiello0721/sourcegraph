FROM grafana/grafana:6.3.6@sha256:2a61d332d673170b227e08acd75160d694b7aaeedd7a39f8ca2fd77a2a2d3cda

LABEL org.opencontainers.image.url=https://sourcegraph.com/
LABEL org.opencontainers.image.source=https://github.com/sourcegraph/sourcegraph/
LABEL org.opencontainers.image.documentation=https://docs.sourcegraph.com/
LABEL com.sourcegraph.grafana.version=6.3.3

# hadolint ignore=DL3020
ADD --chown=grafana:grafana config /sg_config_grafana

# hadolint ignore=DL3020
ADD --chown=grafana:grafana entry.sh /

USER grafana
ENTRYPOINT ["/entry.sh"]
