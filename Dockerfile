FROM openjdk:8-jre-alpine

ENV SONAR_SCANNER_OPTS="-Xmx512m" \
    SONAR_SCANNER_VER="4.0.0.1744" \
    PATH=/opt/sonar-scanner/bin:${PATH}

RUN apk update && \
    apk add --no-cache ca-certificates git && \
    rm -rf /var/cache/apk/* && \
    mkdir /opt

ADD https://sonarsource.bintray.com/Distribution/sonar-scanner-cli/sonar-scanner-cli-${SONAR_SCANNER_VER}.zip /tmp/sonar-scanner-cli-${SONAR_SCANNER_VER}.zip
RUN unzip -q /tmp/sonar-scanner-cli-${SONAR_SCANNER_VER}.zip -d /opt/ && \
    rm -rf /tmp/sonar-scanner-cli-${SONAR_SCANNER_VER}.zip && \
    ln -s /opt/sonar-scanner-${SONAR_SCANNER_VER} /opt/sonar-scanner

COPY sonar-scanner.properties.tmpl /opt/sonar-scanner/conf/sonar-scanner.properties.tmpl
ADD sonar-scanner-plugin /bin/
ENTRYPOINT ["/bin/sonar-scanner-plugin"]
