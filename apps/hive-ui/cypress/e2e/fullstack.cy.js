function unique(prefix) {
  return `${prefix}-${Date.now()}-${Math.floor(Math.random() * 1000)}`
}

function waitForOverview(predicate, attempts = 60) {
  return cy.request('/api/overview').then(({ body }) => {
    if (predicate(body)) {
      return body
    }

    if (attempts <= 1) {
      throw new Error('Timed out waiting for overview state to match expected condition')
    }

    return cy.wait(2000).then(() => waitForOverview(predicate, attempts - 1))
  })
}

describe('Hive UI mobile-first management', () => {
  it('creates projects, drones, and links them', () => {
    const projectName = unique('project')
    const requirementTitle = `Requirement ${Date.now()}`
    const droneName = unique('drone')
    const linkName = unique('link')

    cy.visit('/')
    cy.contains('Vinculum').should('be.visible')

    cy.get('[data-testid="project-name-input"]').clear().type(projectName)
    cy.intercept('POST', '/api/repositories').as('createProject')
    cy.get('[data-testid="create-project-button"]').click()
    cy.wait('@createProject').its('response.statusCode').should('eq', 200)
    cy.contains('Project created.').should('be.visible')
    waitForOverview((body) => body.repositories.some((item) => item.metadata?.name === projectName && item.status?.phase === 'Ready'))

    cy.contains('Requirements').click()
    cy.get('[data-testid="requirement-project-select"]').click()
    cy.contains('li', projectName).click()
    cy.get('[data-testid="requirement-title-input"]').clear().type(requirementTitle)
    cy.get('[data-testid="requirement-body-input"]').clear().type('# Context{enter}{enter}Create a tracked requirement.{enter}{enter}# Acceptance Criteria{enter}{enter}- It exists in the repository.')
    cy.intercept('POST', '/api/requirements').as('createRequirement')
    cy.get('[data-testid="create-requirement-button"]').click()
    cy.wait('@createRequirement').its('response.statusCode').should('eq', 200)
    cy.contains('Requirement created.').should('be.visible')
    cy.request('/api/overview').its('body.requirements').should((items) => {
      expect(items.some((item) => item.spec?.repositoryRef === projectName)).to.eq(true)
    })

    cy.get('[data-testid="nav-row"]').should('exist')
    cy.contains('Drones').click()
    cy.get('[data-testid="drone-name-input"]').clear().type(droneName)
    cy.get('[data-testid="drone-username-input"]').clear().type(`${droneName}-bot`)
    cy.intercept('POST', '/api/drones').as('createDrone')
    cy.get('[data-testid="create-drone-button"]').click()
    cy.wait('@createDrone').its('response.statusCode').should('eq', 200)
    cy.contains('Drone created.').should('be.visible')
    waitForOverview((body) => body.drones.some((item) => item.metadata?.name === droneName && item.status?.forgejoReady === true))

    cy.contains('Projects').click()
    cy.get(`[data-testid="project-link-${projectName}"]`).click()
    cy.contains('Create link').should('be.visible')

    cy.contains('Drones').click()
    cy.get(`[data-testid="drone-link-${droneName}"]`).click()
    cy.contains('Create link').should('be.visible')

    cy.contains('Links').click()
    cy.get('[data-testid="link-name-input"]').clear().type(linkName)
    cy.intercept('POST', '/api/accesses').as('createLink')
    cy.get('[data-testid="create-link-button"]').click()
    cy.wait('@createLink').its('response.statusCode').should('eq', 200)
    cy.contains('Link created.').should('be.visible')
    waitForOverview((body) => body.accesses.some((item) => item.metadata?.name === linkName && item.status?.phase === 'Ready'))

    cy.request('/api/overview').then(({ body }) => {
      expect(body.repositories.some((item) => item.metadata?.name === projectName)).to.eq(true)
      expect(body.requirements.some((item) => item.spec?.repositoryRef === projectName)).to.eq(true)
      expect(body.drones.some((item) => item.metadata?.name === droneName)).to.eq(true)
      expect(body.accesses.some((item) => item.metadata?.name === linkName)).to.eq(true)
    })
  })
})
